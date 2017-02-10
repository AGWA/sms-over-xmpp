package sms // import "github.com/mndrix/sms-over-xmpp"

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

// ErrIgnoreMessage should be returned to indicate that a message
// should be ignored; as if it never happened.
var ErrIgnoreMessage = errors.New("ignore this message")

// Component represents an SMS-over-XMPP component.
type Component struct {
	config Config

	// xmpp is the XMPP component which handles all interactions
	// with an XMPP server.
	xmpp *xco.Component

	// xmppMutex serializes access to the XMPP component to avoid
	// collisions while talking to the XMPP server.
	xmppMutex sync.Mutex

	// receiptFor contains message delivery receipts that
	// haven't been delivered yet.  the key is a provider's outgoing
	// SMS identifier.  the value is the delivery receipt that we should deliver
	// once the associated SMS has been delivered.
	receiptFor map[string]*xco.Message

	// receiptForMutex serializes acces to the receiptFor structure
	receiptForMutex sync.Mutex

	// rxSmsCh is a channel connecting HTTP->gateway.  It communicates
	// information received about SMS (a message, a status update,
	// etc.)
	rxSmsCh chan rxSms
}

// Main runs a component using the given configuration.  It's the main
// entrypoint for launching your own component if you don't want to
// use the sms-over-xmpp command.
func Main(config Config) {
	sc := &Component{config: config}
	sc.receiptFor = make(map[string]*xco.Message)
	sc.rxSmsCh = make(chan rxSms)

	// start processes running
	gatewayDead := sc.runGatewayProcess()
	xmppDead := sc.runXmppComponent()
	httpDead := sc.runHttpProcess()

	for {
		select {
		case _ = <-gatewayDead:
			log.Printf("Gateway died. Restarting")
			gatewayDead = sc.runGatewayProcess()
		case _ = <-httpDead:
			log.Printf("HTTP died. Restarting")
			httpDead = sc.runHttpProcess()
		case _ = <-xmppDead:
			log.Printf("XMPP died. Restarting")
			time.Sleep(1 * time.Second) // don't hammer server
			xmppDead = sc.runXmppComponent()
		}
	}
}

// runGatewayProcess starts the Gateway process. it translates between
// the HTTP and XMPP processes.
func (sc *Component) runGatewayProcess() <-chan struct{} {
	healthCh := make(chan struct{})
	go func(rxSmsCh <-chan rxSms) {
		defer func() { close(healthCh) }()

		for {
			select {
			case rxSms := <-rxSmsCh:
				errCh := rxSms.ErrCh()
				switch x := rxSms.(type) {
				case *rxSmsMessage:
					errCh <- sc.sms2xmpp(x.sms)
				case *rxSmsStatus:
					switch x.status {
					case smsDelivered:
						errCh <- sc.smsDelivered(x.id)
					default:
						log.Panicf("unexpected SMS status: %d", x.status)
					}
				default:
					log.Panicf("unexpected rxSms type: %#v", rxSms)
				}
			}
			log.Println("gateway looping")
		}
	}(sc.rxSmsCh)
	return healthCh
}

// runHttpProcess starts the HTTP agent
func (sc *Component) runHttpProcess() <-chan struct{} {
	config := sc.config

	// choose an SMS provider
	provider, err := config.SmsProvider()
	if err != nil {
		msg := fmt.Sprintf("Couldn't choose an SMS provider: %s", err)
		panic(msg)
	}

	http := &httpProcess{
		host:     config.HttpHost(),
		port:     config.HttpPort(),
		provider: provider,
		rxSmsCh:  sc.rxSmsCh,
		sc:       sc,
	}
	if cfg, ok := config.(CanHttpAuth); ok {
		http.user = cfg.HttpUsername()
		http.password = cfg.HttpPassword()
	}
	return http.run()
}

// runXmppComponent creates a goroutine for sending and receiving XMPP
// stanzas.  it returns a channel for monitoring the goroutine's health.
// if that channel closes, the XMPP goroutine has died.
func (sc *Component) runXmppComponent() <-chan struct{} {
	config := sc.config
	opts := xco.Options{
		Name:         config.ComponentName(),
		SharedSecret: config.SharedSecret(),
		Address:      fmt.Sprintf("%s:%d", config.XmppHost(), config.XmppPort()),
		Logger:       log.New(os.Stderr, "", log.LstdFlags),
	}

	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()

		c, err := xco.NewComponent(opts)
		if err != nil {
			log.Printf("can't create internal XMPP component: %s", err)
			return
		}

		c.MessageHandler = sc.onMessage
		c.DiscoInfoHandler = sc.onDiscoInfo
		c.PresenceHandler = sc.onPresence
		c.IqHandler = sc.onIq
		c.UnknownHandler = sc.onUnknown
		sc.setXmpp(c)

		err = c.Run()
		log.Printf("lost XMPP connection: %s", err)
	}()
	return healthCh
}

func (sc *Component) setXmpp(c *xco.Component) {
	sc.xmppMutex.Lock()
	defer func() { sc.xmppMutex.Unlock() }()

	sc.xmpp = c
}

func (sc *Component) onMessage(c *xco.Component, m *xco.Message) error {
	log.Printf("Message: %+v", m)
	if m.Body == "" {
		log.Printf("  ignoring message with empty body")
		return nil
	}

	// convert recipient address into a phone number
	toPhone, err := sc.config.AddressToPhone(m.To)
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		return nil
	default:
		return errors.Wrap(err, "converting 'to' address to phone")
	}

	// convert author's address into a phone number
	fromPhone, err := sc.config.AddressToPhone(m.From)
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		return nil
	default:
		return errors.Wrap(err, "converting 'from' address to phone")
	}

	// choose an SMS provider
	provider, err := sc.config.SmsProvider()
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		return nil
	default:
		return errors.Wrap(err, "choosing an SMS provider")
	}

	// send the message
	id, err := provider.SendSms(&Sms{
		From: fromPhone,
		To:   toPhone,
		Body: m.Body,
	})
	if err != nil {
		return errors.Wrap(err, "sending SMS")
	}
	log.Printf("Sent SMS with ID %s", id)

	// prepare to handle delivery receipts
	if m.ReceiptRequest != nil && id != "" {
		receipt := xco.Message{
			Header: xco.Header{
				From: m.Header.To,
				To:   m.Header.From,
				ID:   NewId(),
			},
			ReceiptAck: &xco.ReceiptAck{
				Id: m.Header.ID,
			},
			XMLName: m.XMLName,
		}
		sc.receiptForMutex.Lock()
		defer func() { sc.receiptForMutex.Unlock() }()
		if len(sc.receiptFor) > 10 { // don't get too big
			log.Printf("clearing pending receipts queue")
			sc.receiptFor = make(map[string]*xco.Message)
		}
		sc.receiptFor[id] = &receipt
		log.Printf("Waiting to send receipt: %#v", receipt)
	}

	return nil
}

func (sc *Component) onDiscoInfo(c *xco.Component, iq *xco.Iq) ([]xco.DiscoIdentity, []xco.DiscoFeature, error) {
	log.Printf("Disco: %+v", iq)
	ids := []xco.DiscoIdentity{
		{
			Category: "gateway",
			Type:     "sms",
			Name:     "SMS over XMPP",
		},
	}
	features := []xco.DiscoFeature{
		{
			Var: "urn:xmpp:receipts",
		},
	}
	return ids, features, nil
}

func (sc *Component) onPresence(c *xco.Component, p *xco.Presence) error {
	log.Printf("Presence: %+v", p)
	return nil
}

func (sc *Component) onIq(c *xco.Component, iq *xco.Iq) error {
	log.Printf("Iq: %+v", iq)
	return nil
}

func (sc *Component) onUnknown(c *xco.Component, x *xml.StartElement) error {
	log.Printf("Unknown: %+v", x)
	return nil
}

// xmppSend sends a single XML stanza over the XMPP connection.  It
// serializes concurrent access to avoid collisions on the wire.
func (sc *Component) xmppSend(msg interface{}) error {
	sc.xmppMutex.Lock()
	defer func() { sc.xmppMutex.Unlock() }()

	return sc.xmpp.Send(msg)
}

// NewId generates a random string which is suitable as an XMPP stanza
// ID.  The string contains enough entropy to be universally unique.
func NewId() string {
	// generate 128 random bits (6 more than standard UUID)
	bytes := make([]byte, 16)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}

	// convert them to base 32 encoding
	s := base32.StdEncoding.EncodeToString(bytes)
	return strings.ToLower(strings.TrimRight(s, "="))
}
