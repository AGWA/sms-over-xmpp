package sms

import (
	"encoding/json"
	"fmt"
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

	publicUrl *url.URL

	client *http.Client
}

// make sure we implement the right interfaces
var _ SmsProvider = &Twilio{}
var _ CanSmsStatus = &Twilio{}

// represents a response from Twilio's API
type twilioApiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`

	Sid   string   `json:"sid"`
	Flags []string `json:"flags"`
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

func (t *Twilio) SendSms(from, to, body string) (string, error) {
	form := make(url.Values)
	form.Set("To", to)
	form.Set("From", from)
	form.Set("Body", body)
	if t.publicUrl != nil {
		form.Set("StatusCallback", t.publicUrl.String())
	}
	res, err := t.do("Messages", form)
	if err != nil {
		return "", err
	}
	return res.Sid, nil
}

func (t *Twilio) ReceiveSms(r *http.Request) (string, string, string, error) {
	from := r.FormValue("From")
	to := r.FormValue("To")
	body := r.FormValue("Body")

	return from, to, body, nil
}

func (t *Twilio) SmsStatus(r *http.Request) (string, string, bool) {
	id := r.FormValue("MessageSid")
	status := r.FormValue("MessageStatus")
	return id, status, (id != "" && status != "")
}
