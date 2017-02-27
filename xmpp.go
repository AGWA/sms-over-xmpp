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

	// contacted records whether the local and remote JIDs,
	// respectively, have contacted each other during the life of this
	// process.
	contacted map[xco.Address]map[xco.Address]bool
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
				local := &stanza.Header.To
				remote := &stanza.Header.From
				if x.isFirstContact(local, remote) {
					x.hadContact(local, remote)
					p := x.requestSubscription(local, remote)
					go func() { tx <- p }()
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
					stanza = x.presenceAvailable(stanza)
					go func() { tx <- stanza }()
				case "subscribe", "unsubscribe":
					stanzas := x.handleSubscribeUnsubscribe(stanza)
					go func() {
						for _, stanza := range stanzas {
							tx <- stanza
						}
					}()
				}
			case *xml.StartElement:
				log.Printf("Unknown: %+v", stanza)
			default:
				panic(fmt.Sprintf("Unexpected stanza type: %#v", st))
			}
		case stanza := <-x.gatewayTx:
			local := &stanza.Header.From
			remote := &stanza.Header.To
			if x.isFirstContact(local, remote) {
				x.hadContact(local, remote)
				//p := x.requestSubscription(local, remote)
				go func() {
					//tx <- p
					tx <- stanza
				}()
			} else {
				go func() { tx <- stanza }()
			}
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

// isFirstContact returns true if the entities local and remote have
// not had any contact during the life of this process.  Contact
// includes both messages and subscriptions.
func (x *xmppProcess) isFirstContact(local, remote *xco.Address) bool {
	if x.contacted == nil {
		return true
	}

	local = local.Bare()
	remotes := x.contacted[*local]
	if remotes == nil {
		return true
	}

	remote = remote.Bare()
	return !remotes[*remote]
}

// hadContact records the fact that local and remote have contacted
// each other.  It could be for the first time or any subsequent time.
func (x *xmppProcess) hadContact(local, remote *xco.Address) {
	local = local.Bare()
	remote = remote.Bare()
	if x.contacted == nil {
		x.contacted = make(map[xco.Address]map[xco.Address]bool)
	}
	remotes := x.contacted[*local]
	if remotes == nil {
		remotes = make(map[xco.Address]bool)
		x.contacted[*local] = remotes
	}
	remotes[*remote] = true
}

func (x *xmppProcess) presenceAvailable(p *xco.Presence) *xco.Presence {
	stanza := &xco.Presence{
		Header: xco.Header{
			From: p.Header.To,
			To:   p.Header.From,
			ID:   NewId(),
		},
	}
	return stanza
}

func (x *xmppProcess) handleSubscribeUnsubscribe(p *xco.Presence) []*xco.Presence {
	// RFC says to use bare JIDs
	local := (&p.Header.To).Bare()
	remote := (&p.Header.From).Bare()

	stanza := &xco.Presence{
		Header: xco.Header{
			From: *local,
			To:   *remote,
			ID:   NewId(),
		},
	}
	switch p.Type {
	case "subscribe":
		stanza.Type = "subscribed"
		stanzas := []*xco.Presence{
			stanza,
			x.presenceAvailable(p),
		}
		if x.isFirstContact(local, remote) {
			x.hadContact(local, remote)
			//stanzas = append(stanzas, x.requestSubscription(local, remote))
		}
		return stanzas
	case "unsubscribe":
		stanza.Type = "unavailable"
		return []*xco.Presence{stanza}
	}

	return nil
}

func (x *xmppProcess) requestSubscription(local, remote *xco.Address) *xco.Presence {
	stanza := &xco.Presence{
		Header: xco.Header{
			From: *local,
			To:   *remote,
			ID:   NewId(),
		},
		Type: "subscribe",
	}
	return stanza
}
