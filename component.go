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

	// Uppercase Echo Component
	c.MessageHandler = func(c *xco.Component, m *xco.Message) error {
		log.Printf("Message: %+v", m)
		if m.Body == "" {
			log.Printf("  ignoring message with empty body")
			return nil
		}
		resp := &xco.Message{
			Header: xco.Header{
				From: m.To,
				To:   m.From,
				ID:   m.ID,
			},
			Subject: m.Subject,
			Thread:  m.Thread,
			Type:    m.Type,
			Body:    strings.ToUpper(m.Body),
			XMLName: m.XMLName,
		}
		log.Printf("Responding: %+v", resp)
		return errors.Wrap(c.Send(resp), "sending response")
	}
	c.PresenceHandler = func(c *xco.Component, p *xco.Presence) error {
		log.Printf("Presence: %+v", p)
		return nil
	}
	c.IqHandler = func(c *xco.Component, iq *xco.Iq) error {
		log.Printf("Iq: %+v", iq)
		return nil
	}
	c.UnknownHandler = func(c *xco.Component, x *xml.StartElement) error {
		log.Printf("Unknown: %+v", x)
		return nil
	}

	err = c.Run()
	if err != nil {
		log.Printf("ERROR: Run: %s", err)
	}
}
