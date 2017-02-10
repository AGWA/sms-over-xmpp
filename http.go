package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"

	xco "github.com/mndrix/go-xco"
)

// runHttpServer creates a goroutine for receiving HTTP requests.
// it returns a channel for monitoring the goroutine's health.
// if that channel closes, the HTTP goroutine has died.
func (sc *Component) runHttpServer() <-chan struct{} {
	config := sc.config
	addr := fmt.Sprintf("%s:%d", config.HttpHost(), config.HttpPort())
	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()
		err := http.ListenAndServe(addr, sc)
		log.Printf("HTTP server error: %s", err)
	}()
	return healthCh
}

func (sc *Component) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	msgSid := r.FormValue("MessageSid")
	log.Printf("%s %s (%s)", r.Method, r.URL.Path, msgSid)

	// verify HTTP authentication
	if !sc.isHttpAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"sms-over-xmpp\"")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Not authorized")
		return
	}

	// which SMS provider is applicable?
	provider, err := sc.config.SmsProvider()
	switch err {
	case nil:
		// all is well. we'll continue below
	case ErrIgnoreMessage:
		msg := "ignored during provider selection"
		log.Println(msg)
		return
	default:
		msg := fmt.Sprintf("ERROR: choosing an SMS provider: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}

	// is this an SMS Status update?
	if p, ok := provider.(CanSmsStatus); ok {
		if smsId, status, ok := p.SmsStatus(r); ok {
			if status == "delivered" {
				sc.receiptForMutex.Lock()
				defer func() { sc.receiptForMutex.Unlock() }()
				if receipt, ok := sc.receiptFor[smsId]; ok {
					err := sc.xmppSend(receipt)
					if err != nil {
						log.Printf("ERROR sending SMS delivery receipt: %s", err)
						return
					}
					log.Printf("Sent SMS delivery receipt")
					delete(sc.receiptFor, smsId)
				}
			}
			return
		}
	}

	fromPhone, toPhone, body, err := provider.ReceiveSms(r)
	if err != nil {
		log.Printf("ERROR receiving SMS: %s", err)
		return
	}

	// convert author's phone number into XMPP address
	from, err := sc.config.PhoneToAddress(fromPhone)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on From address"
		log.Println(msg)
		return
	default:
		msg := fmt.Sprintf("ERROR: From address %s: %s", fromPhone, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}

	// convert recipient's phone number into XMPP address
	to, err := sc.config.PhoneToAddress(toPhone)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on To address"
		log.Println(msg)
		return
	default:
		msg := fmt.Sprintf("ERROR: To address %s: %s", toPhone, err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
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
		Body: body,
	}
	err = sc.xmppSend(msg)
	if err != nil {
		log.Printf("ERROR: can't send message: %s", err)
	}
}

func (sc *Component) isHttpAuthenticated(r *http.Request) bool {
	// config without any HTTP auth allows everything
	conf, ok := sc.config.(CanHttpAuth)
	if !ok {
		return true
	}
	wantUser := conf.HttpUsername()
	wantPass := conf.HttpPassword()
	if wantUser == "" && wantPass == "" {
		return true
	}

	// now we know that HTTP authentication is mandatory
	gotUser, gotPass, ok := r.BasicAuth()
	if !ok {
		return false
	}

	return gotUser == wantUser && gotPass == wantPass
}
