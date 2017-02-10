package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

// httpAgent is the piece which listens for incoming HTTP requests and
// converts them into values which the rest of the system can
// understand.
type httpAgent struct {
	// where to listen for incoming HTTP requests
	host string
	port int

	// credentials for HTTP auth
	user     string
	password string

	// this field is only temporary. remove after refactoring
	sc *Component
}

// run creates a goroutine for receiving HTTP requests.  It returns a
// channel for monitoring the goroutine's health.  If that channel
// closes, the HTTP goroutine has died.
func (h *httpAgent) run() <-chan struct{} {
	addr := fmt.Sprintf("%s:%d", h.host, h.port)
	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()
		err := http.ListenAndServe(addr, h)
		log.Printf("HTTP server error: %s", err)
	}()
	return healthCh
}

func (h *httpAgent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sc := h.sc
	msgSid := r.FormValue("MessageSid")
	log.Printf("%s %s (%s)", r.Method, r.URL.Path, msgSid)

	// verify HTTP authentication
	if !h.isHttpAuthenticated(r) {
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

	err = sc.sms2xmpp(fromPhone, toPhone, body)
	if err != nil {
		msg := fmt.Sprintf("ERROR: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}
}

func (sc *Component) sms2xmpp(fromPhone, toPhone, body string) error {

	// convert author's phone number into XMPP address
	from, err := sc.config.PhoneToAddress(fromPhone)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on From address"
		log.Println(msg)
		return nil
	default:
		return errors.Wrap(err, "From address "+fromPhone)
	}

	// convert recipient's phone number into XMPP address
	to, err := sc.config.PhoneToAddress(toPhone)
	switch err {
	case nil:
		// all is well. proceed
	case ErrIgnoreMessage:
		msg := "ignored based on To address"
		log.Println(msg)
		return nil
	default:
		return errors.Wrap(err, "To address "+toPhone)
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
	return errors.Wrap(err, "can't send message")
}

func (h *httpAgent) isHttpAuthenticated(r *http.Request) bool {
	wantUser := h.user
	wantPass := h.password
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
