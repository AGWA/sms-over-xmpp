package sms

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"strings"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

// xmppProcess is the piece which interacts with the XMPP network and
// converts those communications into values which the rest of the
// system can understand.
type xmppProcess struct {
	// where to connect to the XMPP server
	host string
	port int

	// credentials for XMPP auth
	name   string
	secret string

	// channel for sending XMPP stanzas to server
	tx chan<- interface{}
}

// runXmppComponent creates a goroutine for sending and receiving XMPP
// stanzas.  it returns a channel for monitoring the goroutine's
// health.  if that channel closes, the XMPP process has died.
func (sc *Component) runXmppComponent(x *xmppProcess) <-chan struct{} {
	opts := xco.Options{
		Name:         x.name,
		SharedSecret: x.secret,
		Address:      fmt.Sprintf("%s:%d", x.host, x.port),
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

		sc.setXmpp(c)
		tx, rx, errx := c.RunAsync()
		x.tx = tx
		for {
			select {
			case stanza := <-rx:
				switch st := stanza.(type) {
				case *xco.Message:
					log.Printf("Message: %+v", st)
					if st.Body == "" {
						log.Printf("  ignoring message with empty body")
						break
					}
					err = sc.onMessage(st)
				case *xco.Presence:
					log.Printf("Presence: %+v", st)
				case *xco.Iq:
					if st.IsDiscoInfo() {
						var ids []xco.DiscoIdentity
						var features []xco.DiscoFeature
						ids, features, err = x.onDiscoInfo(st)
						if err == nil {
							st, err = st.DiscoInfoReply(ids, features)
							if err == nil {
								go func() { tx <- st }()
							}
						}
					} else {
						log.Printf("Iq: %+v", st)
					}
				case *xml.StartElement:
					log.Printf("Unknown: %+v", st)
				default:
					panic(fmt.Sprintf("Unexpected stanza type: %#v", stanza))
				}
			case err = <-errx:
			}

			if err != nil {
				break
			}
		}

		log.Printf("lost XMPP connection: %s", err)
	}()
	return healthCh
}

func (sc *Component) setXmpp(c *xco.Component) {
	sc.xmppMutex.Lock()
	defer func() { sc.xmppMutex.Unlock() }()

	sc.xmpp = c
}

func (sc *Component) onMessage(m *xco.Message) error {
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

func (x *xmppProcess) onDiscoInfo(iq *xco.Iq) ([]xco.DiscoIdentity, []xco.DiscoFeature, error) {
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
