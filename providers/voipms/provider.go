/*
 * Copyright (c) 2022 Andrew Ayer
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * Except as contained in this notice, the name(s) of the above copyright
 * holders shall not be used in advertising or otherwise to promote the
 * sale, use or other dealings in this Software without prior written
 * authorization.
 */

package voipms

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"io"
	"encoding/json"

	"src.agwa.name/sms-over-xmpp"
	"src.agwa.name/sms-over-xmpp/httputil"
)

type Provider struct {
	service *smsxmpp.Service

	apiUsername  string
	apiPassword  string
	httpPassword string
}

func (provider *Provider) Type() string {
	return "voipms"
}

func (provider *Provider) Send(message *smsxmpp.Message) error {
	from, ok := strings.CutPrefix(message.From, "+1")
	if !ok {
		return fmt.Errorf("voip.ms cannot send SMS from %q - only phone numbers with +1 country code are supported", message.From)
	}
	to, ok := strings.CutPrefix(message.To, "+1")
	if !ok {
		return fmt.Errorf("voip.ms cannot send SMS to %q - only phone numbers with +1 country code are supported", message.To)
	}

	request := make(url.Values)
	request.Set("api_username", provider.apiUsername)
	request.Set("api_password", provider.apiPassword)
	request.Set("content_type", "json")
	request.Set("did", from)
	request.Set("dst", to)
	request.Set("message", message.Body)
	if len(message.MediaURLs) >= 1 {
		request.Set("media1", message.MediaURLs[0])
	}
	if len(message.MediaURLs) >= 2 {
		request.Set("media2", message.MediaURLs[1])
	}
	if len(message.MediaURLs) >= 3 {
		request.Set("media3", message.MediaURLs[2])
	}

	if len(message.Body) <= 160 && len(message.MediaURLs) == 0 {
		request.Set("method", "sendSMS")
	} else if len(message.Body) <= 2048 && len(message.MediaURLs) <= 3 {
		request.Set("method", "sendMMS")
	} else {
		return errors.New("Message too long (voip.ms messages must be <= 2048 bytes long and have <= 3 attachments)")
	}

	if resp, err := doRequest(request); err != nil {
		return err
	} else if resp.Status != "success" {
		return fmt.Errorf("sending SMS failed with status %q", resp.Status)
	}

	return nil
}

func (provider *Provider) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sms", provider.handleSMS)
	return httputil.RequireHTTPAuthHandler(provider.httpPassword, mux)
}

func (provider *Provider) handleSMS(w http.ResponseWriter, req *http.Request) {
	requestBytes, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "400 Bad Request: unable to read request body", 400)
		return
	}
	log.Printf("voipms: received webhook with POST body %q", string(requestBytes))
	var requestDoc struct {
		Data struct {
			Payload struct {
				From struct {
					PhoneNumber string `json:"phone_number"`
				} `json:"from"`
				To []struct {
					PhoneNumber string `json:"phone_number"`
				} `json:"to"`
				Text string `json:"text"`
				Media []struct{
					URL string `json:"url"`
				} `json:"media"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal(requestBytes, &requestDoc); err != nil {
		log.Printf("voipms: ignoring inbound webhook due to malformed JSON: %s", err)
		http.Error(w, "400 Bad Request: malformed JSON", 400)
		return
	}
	payload := &requestDoc.Data.Payload
	if len(payload.To) != 1 {
		log.Printf("voipms: ignoring inbound webhook because it has %d destination phone numbers nstead of 1", len(payload.To))
		http.Error(w, "400 Bad Request: number of destination phone numbers is not 1", 400)
		return
	}
	message := smsxmpp.Message{
		From: "+1" + payload.From.PhoneNumber,
		To:   "+1" + payload.To[0].PhoneNumber,
		Body: payload.Text,
	}
	for _, media := range payload.Media {
		message.MediaURLs = append(message.MediaURLs, media.URL)
	}
	if err := provider.service.Receive(&message); err != nil {
		log.Printf("voipms: unable to process inbound SMS from %s to %s: %s", message.From, message.To, err)
		http.Error(w, "500 Internal Server Error: failed to receive message", 500)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
	fmt.Fprintln(w, "ok") // voip.ms requires exactly this response
}

func MakeProvider(service *smsxmpp.Service, config smsxmpp.ProviderConfig) (smsxmpp.Provider, error) {
	return &Provider{
		service:      service,
		apiUsername:  config["api_username"],
		apiPassword:  config["api_password"],
		httpPassword: config["http_password"],
	}, nil
}

func init() {
	smsxmpp.RegisterProviderType("voipms", MakeProvider)
}
