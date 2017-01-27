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
	authToken  string

	client *http.Client
}

// make sure we implement the interface
var _ SmsProvider = &Twilio{}

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
func (t *Twilio) do(service string, form url.Values) error {
	url := "https://api.twilio.com/2010-04-01/Accounts/" + t.accountSid + "/" + service + ".json"
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.Wrap(err, "building HTTP request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(t.accountSid, t.authToken)

	res, err := t.httpClient().Do(req)
	if err != nil {
		return errors.Wrap(err, "running HTTP request")
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("unexpected status: %s", res.Status)
	}

	// parse response
	tRes := new(twilioApiResponse)
	defer res.Body.Close()
	err = json.NewDecoder(res.Body).Decode(tRes)
	if err != nil {
		return errors.Wrap(err, "parsing Twilio response")
	}

	// was the message queued for delivery?
	if tRes.Status != "queued" {
		return fmt.Errorf("unexpected status from Twilio API (%s): %s", tRes.Status, tRes.Message)
	}

	return nil
}

func (t *Twilio) SendSms(from, to, body string) error {
	form := make(url.Values)
	form.Set("To", to)
	form.Set("From", from)
	form.Set("Body", body)
	return t.do("Messages", form)
}
