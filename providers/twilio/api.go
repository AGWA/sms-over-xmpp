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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type apiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`

	Sid   string   `json:"sid"`
	Flags []string `json:"flags"`
}

func (provider *Provider) doTwilioRequest(service string, form url.Values) (*apiResponse, error) {
	url := provider.apiURL + "/2010-04-01/Accounts/" + provider.accountSID + "/" + service + ".json"
	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(provider.keySID, provider.keySecret)

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	respBytes, err := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("Error reading response from Twilio: %s", err)
	}

	if !(httpResp.StatusCode >= 200 && httpResp.StatusCode <= 299) {
		return nil, fmt.Errorf("HTTP error from Twilio: %s: %s", httpResp.Status, respBytes)
	}

	resp := new(apiResponse)
	if err := json.Unmarshal(respBytes, resp); err != nil {
		return nil, err
	}

	if resp.Status != "queued" {
		return nil, fmt.Errorf("Message could not be queued: %s: %s", resp.Status, resp.Message)
	}

	return resp, nil
}
