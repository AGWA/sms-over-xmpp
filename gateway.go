package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"encoding/xml"
	"log"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

func (sc *Component) sms2xmpp(sms *Sms) error {

	// convert author's phone number into XMPP address
	from, err := sc.config.PhoneToAddress(sms.From)
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
	to, err := sc.config.PhoneToAddress(sms.To)
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
	err = sc.xmppSend(msg)
	return errors.Wrap(err, "can't send message")
}

func (sc *Component) smsDelivered(smsId string) error {
	sc.receiptForMutex.Lock()
	defer func() { sc.receiptForMutex.Unlock() }()

	if receipt, ok := sc.receiptFor[smsId]; ok {
		err := sc.xmppSend(receipt)
		if err != nil {
			return errors.Wrap(err, "sending SMS delivery receipt")
		}
		log.Printf("Sent SMS delivery receipt")
		delete(sc.receiptFor, smsId)
	}
	return nil
}
