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
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"log"
	"time"

	"src.agwa.name/sms-over-xmpp/config"
	"src.agwa.name/go-xmpp/component"
	"src.agwa.name/go-xmpp"
)

type user struct {
	phoneNumber string // e.g. "+19255551212"
	provider    Provider
}

type Service struct {
	publicURL       string
	users           map[xmpp.Address]user // Map from bare JID -> user
	providers       map[string]Provider
	xmppParams      component.Params
	xmppSendChan    chan interface{}
}

func NewService(config *config.Config) (*Service, error) {
	service := &Service{
		publicURL:   config.PublicURL,
		users:       make(map[xmpp.Address]user),
		providers:   make(map[string]Provider),
		xmppParams:  component.Params{
			Domain: config.XMPPDomain,
			Secret: config.XMPPSecret,
			Server: config.XMPPServer,
			Logger: log.New(os.Stderr, "", log.Ldate | log.Ltime | log.Lmicroseconds),
		},
		xmppSendChan: make(chan interface{}),
	}

	for providerName, providerConfig := range config.Providers {
		provider, err := MakeProvider(providerConfig.Type, service, providerConfig.Params)
		if err != nil {
			return nil, fmt.Errorf("Provider %s: %s", providerName, err)
		}
		service.providers[providerName] = provider
	}

	for userJID, userConfig := range config.Users {
		userAddress, err := xmpp.ParseAddress(userJID)
		if err != nil {
			return nil, fmt.Errorf("User %s has malformed JID: %s", userJID, err)
		}
		userProvider, providerExists := service.providers[userConfig.Provider]
		if !providerExists {
			return nil, fmt.Errorf("User %s refers to non-existent provider %s", userJID, userConfig.Provider)
		}
		service.users[userAddress] = user{
			phoneNumber: userConfig.PhoneNumber,
			provider: userProvider,
		}
	}
	return service, nil
}

func (service *Service) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	for name, provider := range service.providers {
		if providerHandler := provider.HTTPHandler(); providerHandler != nil {
			mux.Handle("/" + name + "/", http.StripPrefix("/" + name, providerHandler))
		}
	}
	return mux
}

func (service *Service) RunXMPPComponent() error {
	callbacks := component.Callbacks{
		Message: service.receiveXMPPMessage,
		Presence: service.receiveXMPPPresence,
	}

	return component.Run(context.Background(), service.xmppParams, callbacks, service.xmppSendChan)
}

func (service *Service) Receive(message *Message) error {
	address, known := service.addressForPhoneNumber(message.To)
	if !known {
		return errors.New("Unknown phone number " + message.To)
	}

	xmppMessage := xmpp.Message{
		Header: xmpp.Header{
			From: &xmpp.Address{message.From, service.xmppParams.Domain, ""},
			To:   &address,
		},
		Body: message.Body,
		Type: xmpp.CHAT,
	}
	// TODO: if message.MediaURLs is non-empty, include them using XEP-0066

	select {
	case service.xmppSendChan <- xmppMessage:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("Timed out when sending XMPP message")
	}
}

func (service *Service) receiveXMPPMessage(ctx context.Context, xmppMessage *xmpp.Message) error {
	if xmppMessage.From == nil || xmppMessage.To == nil {
		return errors.New("Received malformed XMPP message: From and To not set")
	}
	user, userExists := service.users[*xmppMessage.From.Bare()]
	if !userExists {
		// TODO: more helpful error message: echo back xmppMessage.From.Bare() and say it's not in users map
		return service.sendXMPPError(xmppMessage.To, xmppMessage.From, "Not authorized")
	}

	toPhoneNumber := xmppMessage.To.LocalPart
	if !isValidPhoneNumber(toPhoneNumber) {
		// TODO: more helpful error message: echo back toPhoneNumber and say what format it should be in
		return service.sendXMPPError(xmppMessage.To, xmppMessage.From, "Invalid phone number")
	}

	message := &Message{
		From: user.phoneNumber,
		To:   toPhoneNumber,
		Body: xmppMessage.Body,
	}
	// TODO: if xmppMessage has XEP-0066 URLs, include them in message.MediaURLs

	go func() {
		err := user.provider.Send(message)
		if err != nil {
			// TODO: if sendXMPPError fails, log the error
			service.sendXMPPError(xmppMessage.To, xmppMessage.From, "Sending SMS failed: " + err.Error())
		}
	}()

	return nil
}

func (service *Service) receiveXMPPPresence(ctx context.Context, presence *xmpp.Presence) error {
	if presence.From == nil || presence.To == nil {
		return errors.New("Received malformed XMPP presence: From and To not set")
	}

	if _, userExists := service.users[*presence.From.Bare()]; !userExists {
		return nil
	}

	if presence.Type == xmpp.SUBSCRIBE {
		if err := service.sendXMPPPresence(presence.To, presence.From, xmpp.SUBSCRIBED, ""); err != nil {
			return err
		}
	}

	if presence.Type == xmpp.SUBSCRIBE || presence.Type == xmpp.PROBE {
		var err error
		if isValidPhoneNumber(presence.To.LocalPart) {
			err = service.sendXMPPPresence(presence.To, presence.From, "", "")
		} else {
			err = service.sendXMPPPresence(presence.To, presence.From, "error", "Invalid phone number")
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (service *Service) sendXMPPError(from *xmpp.Address, to *xmpp.Address, message string) error {
	xmppMessage := &xmpp.Message{
		Header: xmpp.Header{
			From: from,
			To:   to,
		},
		Type: xmpp.ERROR,
		Body: message,
	}
	select {
	case service.xmppSendChan <- xmppMessage:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("Timed out when sending XMPP error message")
	}
}

func (service *Service) sendXMPPPresence(from *xmpp.Address, to *xmpp.Address, presenceType string, status string) error {
	xmppPresence := &xmpp.Presence{
		Header: xmpp.Header{
			From: from,
			To:   to,
		},
		Type: presenceType,
		Status: status,
	}
	select {
	case service.xmppSendChan <- xmppPresence:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("Timed out when sending XMPP presence")
	}
}

func (service *Service) addressForPhoneNumber(phoneNumber string) (xmpp.Address, bool) {
	for address, user := range service.users {
		if user.phoneNumber == phoneNumber {
			return address, true
		}
	}
	return xmpp.Address{}, false
}
