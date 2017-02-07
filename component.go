package sms // import "github.com/mndrix/sms-over-xmpp"

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
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
}

// Main runs a component using the given configuration.  It's the main
// entrypoint for launching your own component if you don't want to
// use the sms-over-xmpp command.
func Main(config Config) {
	sc := &Component{config: config}

	// start goroutine for handling XMPP and HTTP
	xmppDead := sc.runXmppComponent()
	httpDead := sc.runHttpServer()

	for {
		select {
		case _ = <-httpDead:
			log.Printf("HTTP died. Restarting")
			httpDead = sc.runHttpServer()
		case _ = <-xmppDead:
			log.Printf("XMPP died. Restarting")
			time.Sleep(1 * time.Second) // don't hammer server
			xmppDead = sc.runXmppComponent()
		}
	}
}

// runHttpServer creates a goroutine for receiving HTTP requests.
// it returns a channel for monitoring the goroutine's health.
// if that channel closes, the HTTP goroutine has died.
func (sc *Component) runHttpServer() <-chan struct{} {
	config := sc.config
	addr := fmt.Sprintf("%s:%d", config.HttpHost(), config.HttpPort())
	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()
		err := http.ListenAndServe(addr, sc)
		log.Printf("HTTP server error: %s", err)
	}()
	return healthCh
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
	err = provider.SendSms(fromPhone, toPhone, m.Body)
	return errors.Wrap(err, "sending SMS")
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
	return ids, nil, nil
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

func (sc *Component) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	msgSid := r.FormValue("MessageSid")
	log.Printf("%s %s (%s)", r.Method, r.URL.Path, msgSid)

	// verify HTTP authentication
	if !sc.isHttpAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"sms-over-xmpp\"")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Not authorized")
		return
	}

	// which SMS provider is applicable?
	provider, err := sc.config.SmsProvider()
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		msg := "ignored during provider selection"
		log.Println(msg)
		return
	default:
		msg := fmt.Sprintf("ERROR: choosing an SMS provider: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}

	fromPhone, toPhone, body, err := provider.ReceiveSms(r)

	// convert author's phone number into XMPP address
	from, err := sc.config.PhoneToAddress(fromPhone)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on From address"
		log.Println(msg)
		return
	default:
		msg := fmt.Sprintf("ERROR: From address %s: %s", fromPhone, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}

	// convert recipient's phone number into XMPP address
	to, err := sc.config.PhoneToAddress(toPhone)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on To address"
		log.Println(msg)
		return
	default:
		msg := fmt.Sprintf("ERROR: To address %s: %s", toPhone, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}

	// deliver message over XMPP
	msg := &xco.Message{
		XMLName: xml.Name{
			Local: "message",
			Space: "jabber:component:accept",
		},

		Header: xco.Header{
			From: from,
			To:   to,
			ID:   NewId(),
		},
		Type: "chat",
		Body: body,
	}
	err = sc.xmppSend(msg)
	if err != nil {
		log.Printf("ERROR: can't send message: %s", err)
	}
}

func (sc *Component) isHttpAuthenticated(r *http.Request) bool {
	// config without any HTTP auth allows everything
	conf, ok := sc.config.(CanHttpAuth)
	if !ok {
		return true
	}
	wantUser := conf.HttpUsername()
	wantPass := conf.HttpPassword()
	if wantUser == "" && wantPass == "" {
		return true
	}

	// now we know that HTTP authentication is mandatory
	gotUser, gotPass, ok := r.BasicAuth()
	if !ok {
		return false
	}

	return gotUser == wantUser && gotPass == wantPass
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
