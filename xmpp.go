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
func (sc *Component) runXmppComponent(
	x *xmppProcess,
	gatewayRx chan<- *xco.Message,
) <-chan struct{} {
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
					go func() { gatewayRx <- st }()
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
