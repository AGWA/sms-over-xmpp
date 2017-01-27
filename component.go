package sms // import "github.com/mndrix/sms-over-xmpp"

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

func Main(config Config) {
	xmppErr := make(chan error)
	go runXmppComponent(config, xmppErr)

	httpErr := make(chan error)
	go runHttpServer(config, httpErr)

	select {
	case err := <-httpErr:
		log.Printf("ERROR HTTP: %s", err)
	case err := <-xmppErr:
		log.Printf("ERROR XMPP: %s", err)
	}
}

func runHttpServer(config Config, errCh chan<- error) {
	addr := fmt.Sprintf("%s:%d", config.HttpHost(), config.HttpPort())
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		fmt.Fprintln(w, "sms-over-xmpp says hi")
	})
	errCh <- http.ListenAndServe(addr, handler)
	close(errCh)
}

func runXmppComponent(config Config, errCh chan<- error) {
	opts := xco.Options{
		Name:         config.ComponentName(),
		SharedSecret: config.SharedSecret(),
		Address:      fmt.Sprintf("%s:%d", config.XmppHost(), config.XmppPort()),
		Logger:       log.New(os.Stderr, "", log.LstdFlags),
	}
	c, err := xco.NewComponent(opts)
	if err != nil {
		panic(err)
	}

	sc := &Component{config}
	c.MessageHandler = sc.onMessage
	c.PresenceHandler = sc.onPresence
	c.IqHandler = sc.onIq
	c.UnknownHandler = sc.onUnknown

	errCh <- c.Run()
	close(errCh)
}

// ErrIgnoreMessage should be returned to indicate that a message
// should be ignored; as if it never happened.
var ErrIgnoreMessage = errors.New("ignore this message")

// Component represents an SMS-over-XMPP component
type Component struct {
	config Config
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
	provider, err := sc.config.SmsProvider(fromPhone, toPhone)
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
