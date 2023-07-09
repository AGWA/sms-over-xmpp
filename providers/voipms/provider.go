package voipms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"src.agwa.name/sms-over-xmpp"
	"src.agwa.name/sms-over-xmpp/httputil"
)

const voipmsApiURL = "https://voip.ms/api/v1/rest.php"

type Provider struct {
	service      *smsxmpp.Service
	apiUrl       string
	apiUsername  string
	apiPassword  string
	httpPassword string
	publicUrl    *url.URL
}

func (provider *Provider) Type() string {
	return "voipms"
}

func (p *Provider) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/message", p.receiveSms)
	return httputil.RequireHTTPAuthHandler(p.httpPassword, mux)
}

func MakeProvider(service *smsxmpp.Service, config smsxmpp.ProviderConfig) (smsxmpp.Provider, error) {
	return &Provider{
		service: service,

		apiUrl: voipmsApiURL,

		// API account at voip.ms
		apiUsername: config["api_username"],
		apiPassword: config["api_password"],

		// Password used by voip.ms' callback URL, to send us
		// messages
		httpPassword: config["callback_password"],
	}, nil
}

type voipmsApiResponse struct {
	Status string `json:"status"`
	Sms    int    `json:"sms"`
}

func (p *Provider) receiveSms(w http.ResponseWriter, r *http.Request) {
	// Will receive: to, from, date, id, message
	id := r.FormValue("id")

	// TODO: How to handle other countries?
	from := "+1" + r.FormValue("from")
	to := "+1" + r.FormValue("to")

	date := r.FormValue("date")
	message := r.FormValue("message")

	media := strings.Split(r.FormValue("media"), ",")
	if len(media) > 0 && media[0] != "" {
		media = nil
	}

	log.Printf("receiveSms: id=%s, date=%s, from=%s, to=%s", id, date, from, to)

	msg := smsxmpp.Message{
		From:      from,
		To:        to,
		Body:      message,
		MediaURLs: media,
	}

	if err := p.service.Receive(&msg); err != nil {
		log.Printf("receiveSms: %v (timestamp=%s, id=%s, from=%s, to=%s)", err, date, id, from, to)
		http.Error(w, "failed to receive message", 500)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(200)
	if _, err := w.Write([]byte("ok")); err != nil {
		log.Printf("receiveSms: w.Write: %v", err)
	}
}

func (p *Provider) Send(msg *smsxmpp.Message) error {
	// GET query parameters:
	// did     => [Required] DID Numbers which is sending the message (Example: 5551234567)
	// dst     => [Required] Destination Number (Example: 5551234568)
	// message => [Required] Message to be sent (Example: 'hello John Smith' max chars: 2048)
	// MMS:
	//   media1 => [Optional] Url to media file (Example: 'https://voip.ms/themes/voipms/assets/img/talent.jpg?v=2')
	//   media2 => [Optional] Base 64 image encode (Example: data:image/png;base64,iVBORw0KGgoAAAANSUh...)
	//   media3 => [Optional] Empty value (Example: '')
	//
	// Requests can be made by the GET and POST methods. When sending
	// multimedia via POST and base64, the file limit is based on the
	// maximum allowed per message, 1.2 mb per file.
	//
	// When sending multimedia via GET and base64, the file limit is based
	// on the maximum allowed by the GET request type, which supports a
	// length of 512 characters, approximately 160kb total weight.
	//
	// In both GET and POST when using file URL submission, this limitation
	// does not exist.
	//
	// Response:
	// Array
	// (
	//     [status] => success
	//     [mms] => 23434
	// )

	u, err := url.Parse(voipmsApiURL)
	q := u.Query()
	q.Set("api_username", p.apiUsername)
	q.Set("api_password", p.apiPassword)

	// TODO: How to handle other countries?
	did := strings.TrimPrefix(msg.From, "+1")
	q.Set("did", did)
	dst := strings.TrimPrefix(msg.To, "+1")
	q.Set("dst", dst)

	q.Set("message", msg.Body)

	method := "sendSMS"
	for i, media := range msg.MediaURLs {
		if i == 3 {
			log.Printf("Send: exceeded maximum of 3 media files, ignoring remaining media URLs")
			break
		}
		q.Set(fmt.Sprintf("media%d", i+1), media)
		method = "sendMMS"
	}
	q.Set("method", method)

	u.RawQuery = q.Encode()

	log.Printf("sendSms: method=%s, to=%s, from=%s", method, dst, did)

	resp, err := http.Get(u.String())
	if err != nil {
		return fmt.Errorf("Send: http.Get: %w", err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		log.Printf("Send: failed to read response body: %v", err)
		return fmt.Errorf("Send: failed to read response body: %w", err)
	}
	body := buf.Bytes()

	var apiResp voipmsApiResponse
	err = json.Unmarshal(body, &apiResp)
	if err != nil {
		log.Printf("Send: parse error in voip.ms response: %v [JSON body: %s]", err, body)
		return fmt.Errorf("Send: parse error in voip.ms response: %w", err)
	}

	// was the message queued for delivery?
	if apiResp.Status != "success" {
		msg := smsxmpp.Message{
			From: msg.From,
			To:   msg.From,
			Body: fmt.Sprintf("ERROR: %s error: %s", method, apiResp.Status),
		}
		if err := p.service.Receive(&msg); err != nil {
			log.Printf("Send: failed to send error message: %v", err)
		}
		return fmt.Errorf("Send: unexpected status from voip.ms API: %s", apiResp.Status)
	}
	return nil
}

func init() {
	smsxmpp.RegisterProviderType("voipms", MakeProvider)
}