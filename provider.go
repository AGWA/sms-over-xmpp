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

package smsxmpp

import (
	"errors"
	"net/http"
	"sync"
)

type Provider interface {
	Type() string
	Send(*Message) error
	HTTPHandler() http.Handler
}

type ProviderConfig map[string]string
type MakeProviderFunc func(*Service, ProviderConfig) (Provider, error)

var (
	providerTypesMu sync.RWMutex
	providerTypes   = make(map[string]MakeProviderFunc)
)

func MakeProvider(typeName string, service *Service, config ProviderConfig) (Provider, error) {
	providerTypesMu.RLock()
	makeProvider, exists := providerTypes[typeName]
	providerTypesMu.RUnlock()

	if !exists {
		return nil, errors.New("Invalid provider type " + typeName)
	}
	return makeProvider(service, config)
}
func RegisterProviderType(name string, makeProvider MakeProviderFunc) {
	providerTypesMu.Lock()
	defer providerTypesMu.Unlock()
	if makeProvider == nil {
		panic("smsxmpp.RegisterProviderType: makeProvider argument is nil")
	}
	if _, alreadyExists := providerTypes[name]; alreadyExists {
		panic("smsxmpp.RegisterProviderType: called twice for type " + name)
	}
	providerTypes[name] = makeProvider
}
