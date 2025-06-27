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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"io"
	"strings"
)

// Inbound SMS request as specified at https://developer.nexmo.com/api/sms#inbound-sms
type inboundSMS struct {
	Msisdn string `json:"msisdn"`
	To     string `json:"to"`
	Text   string `json:"text"`
}

// Response to sending an SMS as specified at https://developer.nexmo.com/api/sms#send-an-sms
type sendSMSResponse struct {
	MessageCount int `json:"message-count"`
	Messages     []struct {
		Status string `json:"status"`
	} `json:"messages"`
}

func (provider *Provider) sendSMS(form url.Values) (*sendSMSResponse, error) {
	req, err := http.NewRequest("POST", "https://rest.nexmo.com/sms/json", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("Error reading response from Nexmo: %s", err)
	}

	if !(httpResp.StatusCode >= 200 && httpResp.StatusCode <= 299) {
		return nil, fmt.Errorf("HTTP error from Nexmo: %s", httpResp.Status)
	}

	resp := new(sendSMSResponse)
	if err := json.Unmarshal(respBytes, resp); err != nil {
		return nil, err
	}

	return resp, nil
}

var sendSMSStatuses = map[string]string{
	"0": "Success",
	"1": "Throttled",
	"2": "Missing Parameters",
	"3": "Invalid Parameters",
	"4": "Invalid Credentials",
	"5": "Internal Error",
	"6": "Invalid Message",
	"7": "Number Barred",
	"8": "Partner Account Barred",
	"9": "Partner Quota Violation",
	"10": "Too Many Existing Binds",
	"11": "Account Not Enabled For HTTP",
	"12": "Message Too Long",
	"14": "Invalid Signature",
	"15": "Invalid Sender Address",
	"22": "Invalid Network Code",
	"23": "Invalid Callback URL",
	"29": "Non-Whitelisted Destination",
	"32": "Signature And API Secret Disallowed",
	"33": "Number De-activated",
}
