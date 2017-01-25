package sms // import "github.com/mndrix/sms-over-xmpp"

import (
	"encoding/xml"
	"fmt"
	"log"
	"strings"

	"github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

func Main(config Config) {
	opts := xco.Options{
		Name:         config.ComponentName(),
		SharedSecret: config.SharedSecret(),
		Address:      fmt.Sprintf("%s:%d", config.XmppHost(), config.XmppPort()),
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

	err = c.Run()
	if err != nil {
		log.Printf("ERROR: Run: %s", err)
	}
}

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
	resp := m.Response()
	resp.Body = strings.ToUpper(m.Body)
	log.Printf("Responding: %+v", resp)

	return errors.Wrap(c.Send(resp), "sending response")
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
