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
