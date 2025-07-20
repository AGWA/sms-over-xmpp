/*
 * Copyright (c) 2019 Andrew Ayer
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

package nexmo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"src.agwa.name/sms-over-xmpp"
	"src.agwa.name/sms-over-xmpp/httputil"
)

type Provider struct {
	service *smsxmpp.Service

	apiKey       string
	apiSecret    string
	httpPassword string
}

func (provider *Provider) Type() string {
	return "nexmo"
}

func (provider *Provider) Send(ctx context.Context, message *smsxmpp.Message) error {
	// https://developer.nexmo.com/api/sms#send-an-sms
	request := make(url.Values)
	request.Set("api_key", provider.apiKey)
	request.Set("api_secret", provider.apiSecret)
	request.Set("from", strings.TrimPrefix(message.From, "+"))
	request.Set("to", strings.TrimPrefix(message.To, "+"))
	request.Set("text", message.Body)
	if !isASCII(message.Body) {
		// TODO: test non-ASCII messages
		request.Set("type", "unicode")
	}

	if len(message.MediaURLs) > 0 {
		return errors.New("Nexmo doesn't support media")
	}

	response, err := provider.sendSMS(ctx, request)
	if err != nil {
		return err
	}

	for _, message := range response.Messages {
		if message.Status != "0" {
			return fmt.Errorf("Error sending SMS (%s): %s", message.Status, sendSMSStatuses[message.Status])
		}
	}

	return nil
}

func (provider *Provider) HTTPHandler() http.Handler {
	// HTTP Basic authentication is supported per https://help.nexmo.com/hc/en-us/articles/230076127-How-to-setup-HTTP-Basic-authentication-for-my-webhook-URL-
	mux := http.NewServeMux()
	mux.HandleFunc("/inbound-sms", provider.handleInboundSMS)
	//mux.HandleFunc("/delivery-receipt", provider.handleDeliveryReceipt) TODO: handle delivery receipts
	return httputil.RequireHTTPAuthHandler(provider.httpPassword, mux)
}

func (provider *Provider) handleInboundSMS(w http.ResponseWriter, req *http.Request) {
	// https://developer.nexmo.com/api/sms#inbound-sms

	requestBytes, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "400 Bad Request: unable to read request body", 400)
		return
	}

	var inboundSMS inboundSMS
	if err := json.Unmarshal(requestBytes, &inboundSMS); err != nil {
		http.Error(w, "400 Bad Request: malformed JSON", 400)
		return
	}

	message := smsxmpp.Message{
		From: "+" + inboundSMS.Msisdn,
		To:   "+" + inboundSMS.To,
		Body: inboundSMS.Text,
	}
	if err := provider.service.Receive(&message); err != nil {
		// TODO: log the error
		http.Error(w, "500 Internal Server Error: failed to receive message", 500)
		return
	}
	w.WriteHeader(204)
}

func MakeProvider(service *smsxmpp.Service, config smsxmpp.ProviderConfig) (smsxmpp.Provider, error) {
	return &Provider{
		service:      service,
		apiKey:       config["api_key"],
		apiSecret:    config["api_secret"],
		httpPassword: config["http_password"],
	}, nil
}

func init() {
	smsxmpp.RegisterProviderType("nexmo", MakeProvider)
}
