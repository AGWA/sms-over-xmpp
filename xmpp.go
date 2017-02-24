package sms

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"

	xco "github.com/mndrix/go-xco"
)

// xmppProcess is the piece which interacts with the XMPP network and
// converts those communications into values which the rest of
// sms-over-xmpp can understand.
type xmppProcess struct {
	// where to connect to the XMPP server
	host string
	port int

	// credentials for XMPP auth
	name   string
	secret string

	// channels for communicating with the Gateway process
	gatewayRx chan<- *xco.Message
	gatewayTx <-chan *xco.Message
}

// runXmppComponent creates a goroutine for sending and receiving XMPP
// stanzas.  it returns a channel for monitoring the goroutine's
// health.  if that channel closes, the XMPP process has died.
func (x *xmppProcess) run() <-chan struct{} {
	opts := xco.Options{
		Name:         x.name,
		SharedSecret: x.secret,
		Address:      fmt.Sprintf("%s:%d", x.host, x.port),
		Logger:       log.New(os.Stderr, "", log.LstdFlags),
	}

	healthCh := make(chan struct{})
	go x.loop(opts, healthCh)
	return healthCh
}

func (x *xmppProcess) loop(opts xco.Options, healthCh chan<- struct{}) {
	defer func() { close(healthCh) }()

	c, err := xco.NewComponent(opts)
	if err != nil {
		log.Printf("can't create internal XMPP component: %s", err)
		return
	}

	tx, rx, errx := c.RunAsync()
	for {
		select {
		case st := <-rx:
			switch stanza := st.(type) {
			case *xco.Message:
				log.Printf("Message: %+v", stanza)
				if stanza.Body == "" {
					log.Printf("  ignoring message with empty body")
					break
				}
				go func() { x.gatewayRx <- stanza }()
			case *xco.Iq:
				if stanza.IsDiscoInfo() {
					log.Printf("Disco: %+v", stanza)
					ids, features := x.describeService()
					stanza, err = stanza.DiscoInfoReply(ids, features)
					if err == nil {
						go func() { tx <- stanza }()
					}
				} else {
					log.Printf("Iq: %+v", stanza)
				}
			case *xco.Presence:
				log.Printf("Presence: %+v", stanza)
				switch stanza.Type {
				case "probe":
					stanza, err = x.presenceAvailable(stanza)
					if err == nil {
						go func() { tx <- stanza }()
					}
				case "subscribe", "unsubscribe":
					var stanzas []*xco.Presence
					stanzas, err = x.handleSubscription(stanza)
					if err == nil {
						go func() {
							for _, stanza := range stanzas {
								tx <- stanza
							}
						}()
					}
				}
			case *xml.StartElement:
				log.Printf("Unknown: %+v", stanza)
			default:
				panic(fmt.Sprintf("Unexpected stanza type: %#v", st))
			}
		case stanza := <-x.gatewayTx:
			go func() { tx <- stanza }()
		case err = <-errx:
		}

		if err != nil {
			break
		}
	}

	log.Printf("lost XMPP connection: %s", err)
}

func (x *xmppProcess) describeService() ([]xco.DiscoIdentity, []xco.DiscoFeature) {
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
	return ids, features
}

func (x *xmppProcess) presenceAvailable(p *xco.Presence) (*xco.Presence, error) {
	stanza := &xco.Presence{
		Header: xco.Header{
			From: p.Header.To,
			To:   p.Header.From,
			ID:   NewId(),
		},
	}
	return stanza, nil
}

func (x *xmppProcess) handleSubscription(p *xco.Presence) ([]*xco.Presence, error) {
	var err error

	// RFC says to use full JIDs
	p.Header.To.ResourcePart = ""
	p.Header.From.ResourcePart = ""

	stanzas := make([]*xco.Presence, 0, 2)
	stanza := &xco.Presence{
		Header: xco.Header{
			From: p.Header.To,
			To:   p.Header.From,
			ID:   NewId(),
		},
	}
	switch p.Type {
	case "subscribe":
		stanza.Type = "subscribed"
		stanzas = append(stanzas, stanza)

		// let user know that we're available
		stanza, err = x.presenceAvailable(p)

		// request a reciprocal subscription
		if err == nil {
			stanzas = append(stanzas, stanza)
			stanza, err = x.requestSubscription(p)
		}
	case "unsubscribe":
		stanza.Type = "unavailable"
	}
	stanzas = append(stanzas, stanza)

	if err != nil {
		return nil, err
	}
	return stanzas, nil
}

func (x *xmppProcess) requestSubscription(p *xco.Presence) (*xco.Presence, error) {
	stanza := &xco.Presence{
		Header: xco.Header{
			From: p.Header.To,
			To:   p.Header.From,
			ID:   NewId(),
		},
		Type: "subscribe",
	}
	return stanza, nil
}
