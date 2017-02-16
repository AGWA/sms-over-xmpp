package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"crypto/rand"
	"encoding/base32"
	"encoding/xml"
	"log"
	"strings"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

// gatewayProcess is the piece which sits between the XMPP and HTTP
// processes, translating between their different protocols.
type gatewayProcess struct {
	// fields shared with Component. see docs there
	config     Config
	receiptFor map[string]*xco.Message
	smsRx      <-chan rxSms
	xmppRx     <-chan *xco.Message
	xmppTx     chan<- *xco.Message
}

func (g *gatewayProcess) run() <-chan struct{} {
	healthCh := make(chan struct{})
	go g.loop(healthCh)
	return healthCh
}

func (g *gatewayProcess) loop(healthCh chan<- struct{}) {
	defer func() { close(healthCh) }()

	for {
		select {
		case rxSms := <-g.smsRx:
			errCh := rxSms.ErrCh()
			switch x := rxSms.(type) {
			case *rxSmsMessage:
				errCh <- g.sms2xmpp(x.sms)
			case *rxSmsStatus:
				switch x.status {
				case smsDelivered:
					err := g.smsDelivered(x.id)
					go func() { errCh <- err }()
				default:
					log.Panicf("unexpected SMS status: %d", x.status)
				}
			default:
				log.Panicf("unexpected rxSms type: %#v", rxSms)
			}
		case msg := <-g.xmppRx:
			err := g.xmpp2sms(msg)
			if err != nil {
				log.Printf("ERROR: converting XMPP to SMS: %s", err)
				return
			}
		}
	}
}

func (g *gatewayProcess) sms2xmpp(sms *Sms) error {

	// convert author's phone number into XMPP address
	from, err := g.config.PhoneToAddress(sms.From)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on From address"
		log.Println(msg)
		return nil
	default:
		return errors.Wrap(err, "From address "+sms.From)
	}

	// convert recipient's phone number into XMPP address
	to, err := g.config.PhoneToAddress(sms.To)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on To address"
		log.Println(msg)
		return nil
	default:
		return errors.Wrap(err, "To address "+sms.To)
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
		Body: sms.Body,
	}
	go func() { g.xmppTx <- msg }()
	return nil
}

func (g *gatewayProcess) smsDelivered(smsId string) error {
	if receipt, ok := g.receiptFor[smsId]; ok {
		go func() { g.xmppTx <- receipt }()
		log.Printf("Sent SMS delivery receipt")
		delete(g.receiptFor, smsId)
	}
	return nil
}

func (g *gatewayProcess) xmpp2sms(m *xco.Message) error {
	// convert recipient address into a phone number
	toPhone, err := g.config.AddressToPhone(m.To)
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		return nil
	default:
		return errors.Wrap(err, "converting 'to' address to phone")
	}

	// convert author's address into a phone number
	fromPhone, err := g.config.AddressToPhone(m.From)
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		return nil
	default:
		return errors.Wrap(err, "converting 'from' address to phone")
	}

	// choose an SMS provider
	provider, err := g.config.SmsProvider()
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		return nil
	default:
		return errors.Wrap(err, "choosing an SMS provider")
	}

	// send the message
	id, err := provider.SendSms(&Sms{
		From: fromPhone,
		To:   toPhone,
		Body: m.Body,
	})
	if err != nil {
		return errors.Wrap(err, "sending SMS")
	}
	log.Printf("Sent SMS with ID %s", id)

	// prepare to handle delivery receipts
	if m.ReceiptRequest != nil && id != "" {
		receipt := xco.Message{
			Header: xco.Header{
				From: m.Header.To,
				To:   m.Header.From,
				ID:   NewId(),
			},
			ReceiptAck: &xco.ReceiptAck{
				Id: m.Header.ID,
			},
			XMLName: m.XMLName,
		}
		if len(g.receiptFor) > 10 { // don't get too big
			log.Printf("clearing pending receipts queue")
			g.receiptFor = make(map[string]*xco.Message)
		}
		g.receiptFor[id] = &receipt
		log.Printf("Waiting to send receipt: %#v", receipt)
	}

	return nil
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
