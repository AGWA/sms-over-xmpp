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

	// channels for communicating with the XMPP server
	xmppTx chan<- interface{}

	// contacted records whether the local and remote JIDs,
	// respectively, have sent a chat message between themselves
	// during the life of this process.  A single message in either
	// direction is enough.
	contacted map[xco.Address]map[xco.Address]bool

	// users records XMPP details about each local user.
	//
	// The map key is the user's bare JID.
	users map[xco.Address]*xmppUser
}

// runXmppComponent creates a goroutine for sending and receiving XMPP
// stanzas.  it returns a channel for monitoring the goroutine's
// health.  if that channel closes, the XMPP process has died.
func (x *xmppProcess) run() <-chan struct{} {
	x.users = make(map[xco.Address]*xmppUser)
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
	x.xmppTx = tx
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
				contact := x.user(local).contact(remote)
				if contact.subTo == no {
					p := x.requestSubscription(local, remote, "")
					x.send(p)
				}
				x.hadContact(local, remote)
				go func() { x.gatewayRx <- stanza }()
			case *xco.Iq:
				if stanza.IsDiscoInfo() {
					log.Printf("Disco: %+v", stanza)
					ids, features := x.describeService()
					stanza, err = stanza.DiscoInfoReply(ids, features)
					if err == nil {
						x.send(stanza)
					}
				} else {
					log.Printf("Iq: %+v", stanza)
				}
			case *xco.Presence:
				log.Printf("Presence: %+v", stanza)
				local := &stanza.Header.To
				remote := &stanza.Header.From
				contact := x.user(local).contact(remote)

				switch stanza.Type {
				case "probe":
					stanza = x.presenceAvailable(stanza)
					x.send(stanza)
				case "subscribe", "unsubscribe":
					stanzas := x.handleSubscribeUnsubscribe(stanza)
					x.send(stanzas...)
				case "subscribed":
					if contact.subTo == pending {
						contact.subTo = yes
					}
				case "unsubscribed":
					contact.subTo = no
				}
			case *xml.StartElement:
				log.Printf("Unknown: %+v", stanza)
			default:
				panic(fmt.Sprintf("Unexpected stanza type: %#v", st))
			}
		case stanza := <-x.gatewayTx:
			local := &stanza.Header.From
			remote := &stanza.Header.To
			contact := x.user(local).contact(remote)

			stanzas := []interface{}{}
			if x.haveRelationship(local, remote) {
				// XEP-0172 says skip nick for existing relationship
				stanza.Nick = ""
			}
			if contact.subTo == no {
				p := x.requestSubscription(local, remote, stanza.Nick)
				stanzas = append(stanzas, p)
			}
			stanzas = append(stanzas, stanza)
			x.send(stanzas...)
		case err = <-errx:
		}

		if err != nil {
			break
		}
	}

	log.Printf("lost XMPP connection: %s", err)
}

// send stanzas to the remote XMPP server.  The transmission happens
// asynchronously.
func (x *xmppProcess) send(stanzas ...interface{}) {
	// bookkeeping for outgoing stanzas
	for _, s := range stanzas {
		switch stanza := s.(type) {
		case *xco.Message:
			local := &stanza.Header.From
			remote := &stanza.Header.To
			x.hadContact(local, remote)
		case *xco.Presence:
			local := &stanza.Header.From
			remote := &stanza.Header.To
			contact := x.user(local).contact(remote)

			switch stanza.Type {
			case "subscribe":
				if contact.subTo == no {
					contact.subTo = pending
				}
			case "unsubscribe":
				contact.subTo = no
			case "subscribed":
				contact.subFrom = yes
			case "unsubscribed":
				contact.subFrom = no
			}
		}
	}

	// perform actual transmission asynchronously
	go func() {
		for _, stanza := range stanzas {
			x.xmppTx <- stanza
		}
	}()
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

// haveRelationship returns true if these contacts have an existing
// XMPP relationship.  That includes a presence subscription in either
// direction or having sent a message between themselves.
func (x *xmppProcess) haveRelationship(local, remote *xco.Address) bool {
	contact := x.user(local).contact(remote)
	return contact.subTo == yes ||
		contact.subFrom == yes ||
		!x.isFirstContact(local, remote)
}

// isFirstContact returns true if the entities local and remote have
// not sent any messages to each other during the life of this
// process.  Only chat messages count, not presence or iq, etc.
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

// hadContact records the fact that a chat message has been sent
// between local and remote.  This could be a single message in either
// direction.
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

func (x *xmppProcess) handleSubscribeUnsubscribe(p *xco.Presence) []interface{} {
	// RFC says to use bare JIDs
	local := (&p.Header.To).Bare()
	remote := (&p.Header.From).Bare()
	contact := x.user(local).contact(remote)

	stanza := &xco.Presence{
		Header: xco.Header{
			From: *local,
			To:   *remote,
			ID:   NewId(),
		},
	}
	switch p.Type {
	case "subscribe":
		if contact.subFrom == no {
			contact.subFrom = pending
		}
		stanza.Type = "subscribed"
		stanzas := []interface{}{
			stanza,
			x.presenceAvailable(p),
		}
		if contact.subTo == no {
			stanzas = append(stanzas, x.requestSubscription(local, remote, ""))
		}
		return stanzas
	case "unsubscribe":
		stanzas := []interface{}{}
		if contact.subFrom == yes {
			stanza.Type = "unavailable"
			stanzas = []interface{}{
				stanza,

				// RFC 6121 A.3.2 says
				// "SHOULD autoreply with unsubscribed stanza"
				&xco.Presence{
					Header: xco.Header{
						From: *local,
						To:   *remote,
						ID:   NewId(),
					},
					Type: "unsubscribed",
				},
			}
		}
		contact.subFrom = no
		return stanzas
	}

	return nil
}

func (x *xmppProcess) requestSubscription(local, remote *xco.Address, nick string) *xco.Presence {
	stanza := &xco.Presence{
		Header: xco.Header{
			From: *local,
			To:   *remote,
			ID:   NewId(),
		},
		Type: "subscribe",
		Nick: nick,
	}
	return stanza
}

// user returns the XMPP user for a local JID.  Creates an empty one
// if none exists.
func (x *xmppProcess) user(a *xco.Address) *xmppUser {
	local := *(a.Bare())
	user, ok := x.users[local]
	if !ok {
		user = &xmppUser{}
		x.users[local] = user
	}
	return user
}
