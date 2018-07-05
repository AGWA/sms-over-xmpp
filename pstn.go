package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"fmt"
	"log"
	"net/http"
)

// pstnProcess is the piece which listens for incoming events from the
// phone network (usually via HTTP requests) and converts them into
// values which the rest of the system can understand.
type pstnProcess struct {
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
}

// run creates a goroutine for receiving PSTN events.  It returns a
// channel for monitoring the goroutine's health.  If that channel
// closes, the PSTN goroutine has died.
func (h *pstnProcess) run() <-chan struct{} {
	addr := fmt.Sprintf("%s:%d", h.host, h.port)
	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()
		err := http.ListenAndServe(addr, h)
		log.Printf("HTTP server error: %s", err)
	}()
	return healthCh
}

func (h *pstnProcess) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// send blank payload for Twilio to stop complaining
	w.Header().Set("Content-Type", "text/xml")
	w.Write([]byte("<Response></Response>"))
}

func (h *pstnProcess) recognizeNotice(r *http.Request, errCh chan<- error) (rxSms, error) {
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

func (h *pstnProcess) isHttpAuthenticated(r *http.Request) bool {
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
