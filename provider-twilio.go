package sms

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

// Twilio represents an account with the communications provider
// Twilio.
type Twilio struct {
	accountSid string
	keySid     string
	keySecret  string

	// where to listen for incoming HTTP requests
	httpHost string
	httpPort int

	// credentials for HTTP auth
	httpUsername string
	httpPassword string

	publicUrl *url.URL

	client *http.Client
}

// make sure we implement the right interfaces
var _ SmsProvider = &Twilio{}

// represents a response from Twilio's API
type twilioApiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`

	Sid   string   `json:"sid"`
	Flags []string `json:"flags"`
}

// represents the HTTP server used for receiving incoming SMS from Twilio
type twilioHttpServer struct {
	username string
	password string
	rxSmsCh  chan<- rxSms
}

func (t *Twilio) httpClient() *http.Client {
	if t.client == nil {
		return http.DefaultClient
	}
	return t.client
}

// do runs a single API request against Twilio
func (t *Twilio) do(service string, form url.Values) (*twilioApiResponse, error) {
	url := "https://api.twilio.com/2010-04-01/Accounts/" + t.accountSid + "/" + service + ".json"
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "building HTTP request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(t.keySid, t.keySecret)

	res, err := t.httpClient().Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "running HTTP request")
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("unexpected status: %s", res.Status)
	}

	// parse response
	tRes := new(twilioApiResponse)
	defer res.Body.Close()
	err = json.NewDecoder(res.Body).Decode(tRes)
	if err != nil {
		return nil, errors.Wrap(err, "parsing Twilio response")
	}

	// was the message queued for delivery?
	if tRes.Status != "queued" {
		return nil, fmt.Errorf("unexpected status from Twilio API (%s): %s", tRes.Status, tRes.Message)
	}

	return tRes, nil
}

func (t *Twilio) SendSms(sms *Sms) (string, error) {
	form := make(url.Values)
	form.Set("To", sms.To)
	form.Set("From", sms.From)
	form.Set("Body", sms.Body)
	if t.publicUrl != nil {
		form.Set("StatusCallback", t.publicUrl.String())
	}
	res, err := t.do("Messages", form)
	if err != nil {
		return "", err
	}
	return res.Sid, nil
}

func (t *Twilio) RunPstnProcess(rxSmsCh chan<- rxSms) <-chan struct{} {
	s := &twilioHttpServer{
		username: t.httpUsername,
		password: t.httpPassword,
		rxSmsCh:  rxSmsCh,
	}
	addr := fmt.Sprintf("%s:%d", t.httpHost, t.httpPort)
	healthCh := make(chan struct{})
	go func() {
		defer func() { close(healthCh) }()
		err := http.ListenAndServe(addr, s)
		log.Printf("HTTP server error: %s", err)
	}()
	return healthCh
}

func (s *twilioHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	msgSid := r.FormValue("MessageSid")
	log.Printf("%s %s (%s)", r.Method, r.URL.Path, msgSid)

	// verify HTTP authentication
	if !s.isHttpAuthenticated(r) {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"sms-over-xmpp\"")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Not authorized")
		return
	}

	// what kind of notice did we receive?
	errCh := make(chan error)
	rx, err := s.recognizeNotice(r, errCh)
	if err == nil && rx != nil {
		s.rxSmsCh <- rx
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

func (s *twilioHttpServer) recognizeNotice(r *http.Request, errCh chan<- error) (rxSms, error) {
	id := r.FormValue("MessageSid")
	status := r.FormValue("MessageStatus")

	if id != "" && status != "" {
		if status == "delivered" {
			rx := &rxSmsStatus{
				id:     id,
				status: smsDelivered,
				errCh:  errCh,
			}
			return rx, nil
		}
		return nil, nil
	}

	from := r.FormValue("From")
	to := r.FormValue("To")
	body := r.FormValue("Body")

	sms := &Sms{
		From: from,
		To:   to,
		Body: body,
	}

	rx := &rxSmsMessage{
		sms:   sms,
		errCh: errCh,
	}
	return rx, nil
}

func (s *twilioHttpServer) isHttpAuthenticated(r *http.Request) bool {
	wantUser := s.username
	wantPass := s.password
	if wantUser == "" && wantPass == "" {
		return true
	}

	// now we know that HTTP authentication is mandatory
	gotUser, gotPass, ok := r.BasicAuth()
	if !ok {
		return false
	}

	if subtle.ConstantTimeCompare([]byte(gotUser), []byte(wantUser)) != 1 {
		return false
	}

	if subtle.ConstantTimeCompare([]byte(gotPass), []byte(wantPass)) != 1 {
		return false
	}

	return true
}
