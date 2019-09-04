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

package twilio

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"src.agwa.name/sms-over-xmpp"
	"src.agwa.name/sms-over-xmpp/httputil"
)

type Provider struct {
	service      *smsxmpp.Service

	apiURL       string
	accountSID   string
	keySID       string
	keySecret    string
	httpPassword string
}

func (provider *Provider) Type() string {
	return "twilio"
}

func (provider *Provider) Send(message *smsxmpp.Message) error {
	request := make(url.Values)
	request.Set("To", message.To)
	request.Set("From", message.From)
	request.Set("Body", message.Body)
	// TODO: callback support: 1. get the public URL for this provider from the smsxmpp.Service; 2. add httpPassword to the URL; 3. set request's "StatusCallback" to URL + "/status_callback"
	if len(message.MediaURLs) > 10 {
		return errors.New("Too many media URLs (Twilio only supports 10 per message)")
	} else if len(message.MediaURLs) > 0 {
		request["MediaUrl"] = message.MediaURLs
	}

	_, err := provider.doTwilioRequest("Messages", request)
	return err
}

func (provider *Provider) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/message", provider.handleMessage)
	//mux.HandleFunc("/status_callback", provider.handleStatusCallback)
	return httputil.RequireHTTPAuthHandler(provider.httpPassword, mux)
}

func (provider *Provider) handleMessage(w http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		http.Error(w, "400 Bad Request: Parsing form failed: " + err.Error(), 400)
		return
	}

	message := smsxmpp.Message{
		From: req.PostForm.Get("From"),
		To: req.PostForm.Get("To"),
		Body: req.PostForm.Get("Body"),
		MediaURLs: getMediaURLs(req.PostForm),
	}
	if err := provider.service.Receive(&message); err != nil {
		// TODO: log the error
		http.Error(w, "500 Internal Server Error: failed to receive message", 500)
		return
	}
	// Note: while Twilio is OK with a 204 response, SignalWire requires this
	// exact response document (XML declaration and non-self-closing <Response>),
	// which fortunately works with Twilio also.
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(w, `<Response></Response>`)
}

func getMediaURLs(form url.Values) []string {
	numMedia, err := strconv.Atoi(form.Get("NumMedia"))
	if err != nil || numMedia == 0 {
		return nil
	}

	mediaURLs := make([]string, numMedia)
	for i := range mediaURLs {
		mediaURLs[i] = form.Get("MediaUrl" + strconv.Itoa(i))
	}
	return mediaURLs
}

func MakeProvider(service *smsxmpp.Service, config smsxmpp.ProviderConfig) (smsxmpp.Provider, error) {
	return &Provider{
		service: service,
		apiURL: "https://api.twilio.com",
		accountSID: config["account_sid"],
		keySID: config["key_sid"],
		keySecret: config["key_secret"],
		httpPassword: config["http_password"],
	}, nil
}

func MakeSignalwireProvider(service *smsxmpp.Service, config smsxmpp.ProviderConfig) (smsxmpp.Provider, error) {
	return &Provider{
		service: service,
		apiURL: "https://" + config["domain"] + "/api/laml",
		accountSID: config["project_id"],
		keySID: config["project_id"],
		keySecret: config["auth_token"],
		httpPassword: config["http_password"],
	}, nil
}

func init() {
	smsxmpp.RegisterProviderType("twilio", MakeProvider)
	smsxmpp.RegisterProviderType("signalwire", MakeSignalwireProvider)
}
