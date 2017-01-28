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

// ErrIgnoreMessage should be returned to indicate that a message
// should be ignored; as if it never happened.
var ErrIgnoreMessage = errors.New("ignore this message")

// Component represents an SMS-over-XMPP component
type Component struct {
	config Config
}

func Main(config Config) {
	sc := &Component{config}

	xmppErr := make(chan error)
	go sc.runXmppComponent(xmppErr)

	httpErr := make(chan error)
	go sc.runHttpServer(httpErr)

	select {
	case err := <-httpErr:
		log.Printf("ERROR HTTP: %s", err)
	case err := <-xmppErr:
		log.Printf("ERROR XMPP: %s", err)
	}
}

func (sc *Component) runHttpServer(errCh chan<- error) {
	config := sc.config
	addr := fmt.Sprintf("%s:%d", config.HttpHost(), config.HttpPort())
	errCh <- http.ListenAndServe(addr, sc)
	close(errCh)
}

func (sc *Component) runXmppComponent(errCh chan<- error) {
	config := sc.config
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

	c.MessageHandler = sc.onMessage
	c.PresenceHandler = sc.onPresence
	c.IqHandler = sc.onIq
	c.UnknownHandler = sc.onUnknown

	errCh <- c.Run()
	close(errCh)
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

	log.Printf("would have sent (%s -> %s): %s", &from, &to, body)
}
