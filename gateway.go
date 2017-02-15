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

func (sc *Component) xmpp2sms(m *xco.Message) error {
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
		sc.receiptForMutex.Lock()
		defer func() { sc.receiptForMutex.Unlock() }()
		if len(sc.receiptFor) > 10 { // don't get too big
			log.Printf("clearing pending receipts queue")
			sc.receiptFor = make(map[string]*xco.Message)
		}
		sc.receiptFor[id] = &receipt
		log.Printf("Waiting to send receipt: %#v", receipt)
	}

	return nil
}
