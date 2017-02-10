package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

// httpProcess is the piece which listens for incoming HTTP requests and
// converts them into values which the rest of the system can
// understand.
type httpProcess struct {
	// where to listen for incoming HTTP requests
	host string
	port int

	// credentials for HTTP auth
	user     string
	password string

	provider SmsProvider

	// rxSmsCh is a channel down which we send information we've
	// received about SMS.
	rxSmsCh chan<- rxSms

	// this field is only temporary. remove after refactoring
	sc *Component
}

// run creates a goroutine for receiving HTTP requests.  It returns a
// channel for monitoring the goroutine's health.  If that channel
// closes, the HTTP goroutine has died.
func (h *httpProcess) run() <-chan struct{} {
	addr := fmt.Sprintf("%s:%d", h.host, h.port)
	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()
		err := http.ListenAndServe(addr, h)
		log.Printf("HTTP server error: %s", err)
	}()
	return healthCh
}

func (h *httpProcess) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	msgSid := r.FormValue("MessageSid")
	log.Printf("%s %s (%s)", r.Method, r.URL.Path, msgSid)

	// verify HTTP authentication
	if !h.isHttpAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"sms-over-xmpp\"")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Not authorized")
		return
	}

	// what kind of notice did we receive?
	errCh := make(chan error)
	rx, err := h.recognizeNotice(r, errCh)
	if err == nil && rx != nil {
		h.rxSmsCh <- rx
		err = <-errCh
	}
	if err != nil {
		msg := fmt.Sprintf("ERROR: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, msg)
		log.Println(msg)
		return
	}
}

func (h *httpProcess) recognizeNotice(r *http.Request, errCh chan<- error) (rxSms, error) {
	if p, ok := h.provider.(CanSmsStatus); ok {
		if smsId, status, ok := p.SmsStatus(r); ok {
			if status == "delivered" {
				rx := &rxSmsStatus{
					id:     smsId,
					status: smsDelivered,
					errCh:  errCh,
				}
				return rx, nil
			}
			return nil, nil
		}
	}

	if sms, err := h.provider.ReceiveSms(r); err == nil {
		rx := &rxSmsMessage{
			sms:   sms,
			errCh: errCh,
		}
		return rx, nil
	} else {
		return nil, err
	}
}

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

func (h *httpProcess) isHttpAuthenticated(r *http.Request) bool {
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
