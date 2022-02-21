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
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"github.com/emersion/go-webdav/carddav"

	"src.agwa.name/sms-over-xmpp/config"
	"src.agwa.name/go-xmpp/component"
	"src.agwa.name/go-xmpp"
)

type RosterItem struct {
	Name   string
	Groups []string
}

func (item RosterItem) Equal(other RosterItem) bool {
	if item.Name != other.Name {
		return false
	}
	if len(item.Groups) != len(other.Groups) {
		return false
	}
	for i := range item.Groups {
		if item.Groups[i] != other.Groups[i] {
			return false
		}
	}
	return true
}

type Roster map[xmpp.Address]RosterItem

var ErrRosterNotIntialized = errors.New("the roster for this user has not been initialized yet")

const rosterSyncInterval = 15*time.Second

type rosterUser struct {
	carddavURL string

	syncChan chan struct{}
	rosterMu sync.Mutex
	roster   Roster
}

func (roster *rosterUser) forceSync() {
	select {
	case roster.syncChan <- struct{}{}:
	default:
	}
}

type user struct {
	phoneNumber string // e.g. "+19255551212"
	provider    Provider
}

type Service struct {
	defaultPrefix   string // prepended to phone numbers that don't start with +
	publicURL       string
	users           map[xmpp.Address]user // Map from bare JID -> user
	rosterUsers     map[xmpp.Address]*rosterUser // Map from bare JID -> *rosterUser
	providers       map[string]Provider
	xmppParams      component.Params
	xmppSendChan    chan interface{}
}

func NewService(config *config.Config) (*Service, error) {
	if config.DefaultPrefix != "" {
		if err := validatePhoneNumber(config.DefaultPrefix); err != nil {
			return nil, fmt.Errorf("default_prefix option is invalid: %s", err)
		}
	}
	service := &Service{
		defaultPrefix: config.DefaultPrefix,
		publicURL:   config.PublicURL,
		users:       make(map[xmpp.Address]user),
		rosterUsers: make(map[xmpp.Address]*rosterUser),
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

	for userJID, carddavURL := range config.Rosters {
		userAddress, err := xmpp.ParseAddress(userJID)
		if err != nil {
			return nil, fmt.Errorf("User %s has malformed JID: %s", userJID, err)
		}
		service.rosterUsers[userAddress] = &rosterUser{
			carddavURL: carddavURL,
			syncChan:   make(chan struct{}, 1),
		}
	}

	return service, nil
}

func (service *Service) sendWithin(timeout time.Duration, v interface{}) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case service.xmppSendChan <- v:
		return true
	case <-timer.C:
		return false
	}
}

func (service *Service) defaultHTTPHandler(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/" {
		http.Error(w, "You have successfully reached sms-over-xmpp.", 200)
	} else {
		http.Error(w, "You have reached sms-over-xmpp, but the provider indicated in the URL is not known.", 404)
	}
}

func (service *Service) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	for name, provider := range service.providers {
		if providerHandler := provider.HTTPHandler(); providerHandler != nil {
			mux.Handle("/" + name + "/", http.StripPrefix("/" + name, providerHandler))
		}
	}
	mux.HandleFunc("/", service.defaultHTTPHandler)
	return mux
}

func (service *Service) RunXMPPComponent(ctx context.Context) error {
	callbacks := component.Callbacks{
		Message: service.receiveXMPPMessage,
		Presence: service.receiveXMPPPresence,
		Iq: service.receiveXMPPIq,
	}

	return component.Run(ctx, service.xmppParams, callbacks, service.xmppSendChan)
}

func (service *Service) RunAddressBookUpdater(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	for userJID, user := range service.rosterUsers {
		userJID, user := userJID, user
		group.Go(func() error {
			return service.runAddressBookUpdaterFor(ctx, userJID, user)
		})
	}
	return group.Wait()
}

func (service *Service) runAddressBookUpdaterFor(ctx context.Context, userJID xmpp.Address, user *rosterUser) error {
	if err := service.sendXMPPRosterQuery(xmpp.RandomID(), userJID, "get", xmpp.RosterQuery{}); err != nil {
		return fmt.Errorf("unable to query roster for %s: %w", userJID, err)
	}
	client, err := carddav.NewClient(http.DefaultClient, user.carddavURL)
	if err != nil {
		return fmt.Errorf("unable to create CardDAV client for %s: %w", userJID, err)
	}
	addrbook := new(addressBook)
	for {
		if err := addrbook.download(ctx, client); err != nil {
			log.Printf("Error downloading address book for %s: %s", userJID, err)
		}
		if addrbook.changed {
			newRoster := addrbook.makeRoster(service.xmppParams.Domain)
			log.Printf("%s: Setting roster = %#v", userJID, newRoster)
			if err := service.setRoster(ctx, userJID, user, newRoster); err == nil {
				addrbook.changed = false
			} else if err != ErrRosterNotIntialized {
				log.Printf("Error setting roster for %s: %s", userJID, err)
			}
		}

		timeout := time.NewTimer(rosterSyncInterval)
		select {
		case <-timeout.C:
		case <-user.syncChan:
			timeout.Stop()
		case <-ctx.Done():
			timeout.Stop()
			return ctx.Err()
		}
	}
}

func (service *Service) Receive(message *Message) error {
	address, known := service.addressForPhoneNumber(message.To)
	if !known {
		return errors.New("Unknown phone number " + message.To)
	}
	from := xmpp.Address{service.friendlyPhoneNumber(message.From), service.xmppParams.Domain, ""}

	if err := service.sendXMPPChat(from, address, message.Body); err != nil {
		return err
	}

	for _, mediaURL := range message.MediaURLs {
		if err := service.sendXMPPMediaURL(from, address, mediaURL); err != nil {
			return err
		}
	}

	return nil
}

func (service *Service) sendXMPPChat(from xmpp.Address, to xmpp.Address, body string) error {
	xmppMessage := xmpp.Message{
		Header: xmpp.Header{
			From: &from,
			To:   &to,
		},
		Body: body,
		Type: xmpp.CHAT,
	}

	if !service.sendWithin(5*time.Second, xmppMessage) {
		return errors.New("Timed out when sending XMPP message")
	}
	return nil
}

func (service *Service) sendXMPPMediaURL(from xmpp.Address, to xmpp.Address, mediaURL string) error {
	xmppMessage := xmpp.Message{
		Header: xmpp.Header{
			From: &from,
			To:   &to,
		},
		Body: mediaURL,
		Type: xmpp.CHAT,
		OutOfBandData: &xmpp.OutOfBandData{URL: mediaURL},
	}

	if !service.sendWithin(5*time.Second, xmppMessage) {
		return errors.New("Timed out when sending XMPP message with out-of-band data")
	}
	return nil
}

func shouldForwardMessageType(t xmpp.MessageType) bool {
	return t == "" || t == xmpp.CHAT || t == xmpp.NORMAL
}

func messageHasContent(message *xmpp.Message) bool {
	// This function filters out "$user is typing" messages
	return message.Body != "" || message.OutOfBandData != nil
}

func shouldForwardMessage(message *xmpp.Message) bool {
	return shouldForwardMessageType(message.Type) && messageHasContent(message)
}

func (service *Service) receiveXMPPMessage(ctx context.Context, xmppMessage *xmpp.Message) error {
	if xmppMessage.From == nil || xmppMessage.To == nil {
		return errors.New("Received malformed XMPP message: From and To not set")
	}
	if !shouldForwardMessage(xmppMessage) {
		return nil
	}
	user, userExists := service.users[*xmppMessage.From.Bare()]
	if !userExists {
		return service.sendXMPPError(xmppMessage.To, xmppMessage.From, xmppMessage.From.Bare().String() + " is not a known user; please add them to sms-over-xmpp's users file")
	}

	toPhoneNumber, err := service.canonPhoneNumber(xmppMessage.To.LocalPart)
	if err != nil {
		return service.sendXMPPError(xmppMessage.To, xmppMessage.From, fmt.Sprintf("Invalid phone number '%s': %s (example: +12125551212)", xmppMessage.To.LocalPart, err))
	}

	message := &Message{
		From: user.phoneNumber,
		To:   toPhoneNumber,
	}
	if xmppMessage.OutOfBandData != nil {
		message.MediaURLs = append(message.MediaURLs, xmppMessage.OutOfBandData.URL)
	} else {
		message.Body = xmppMessage.Body
	}

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
		var presenceType string
		var status string

		if _, err := service.canonPhoneNumber(presence.To.LocalPart); err != nil {
			presenceType = "error"
			status = "Invalid phone number: " + err.Error()
		}

		if err := service.sendXMPPPresence(presence.To, presence.From, presenceType, status); err != nil {
			return err
		}
	}

	return nil
}

func (service *Service) receiveXMPPIq(ctx context.Context, iq *xmpp.Iq) error {
	switch {
	case iq.RosterQuery != nil:
		return service.receiveXMPPRosterQuery(ctx, iq)
	default:
		return nil
	}
}

func (service *Service) receiveXMPPRosterQuery(ctx context.Context, iq *xmpp.Iq) error {
	if iq.From == nil || iq.To == nil {
		return errors.New("Received malformed XMPP iq: From and To not set")
	}

	user, userExists := service.rosterUsers[*iq.From.Bare()]
	if !userExists {
		return nil
	}

	switch iq.Type {
	case "set":
		return service.receiveXMPPRosterSet(ctx, user, iq.RosterQuery)
	case "result":
		return service.receiveXMPPRosterResult(ctx, user, iq.RosterQuery)
	default:
		return nil
	}
}

func (service *Service) receiveXMPPRosterSet(ctx context.Context, user *rosterUser, query *xmpp.RosterQuery) error {
	user.rosterMu.Lock()
	defer user.rosterMu.Unlock()

	if user.roster == nil {
		return nil
	}
	if len(query.Items) != 1 {
		return nil
	}
	item := query.Items[0]
	if item.Subscription == "remove" {
		delete(user.roster, item.JID)
	} else {
		user.roster[item.JID] = RosterItem{
			Name:   item.Name,
			Groups: item.Groups,
		}
	}

	return nil
}

func (service *Service) receiveXMPPRosterResult(ctx context.Context, user *rosterUser, query *xmpp.RosterQuery) error {
	user.rosterMu.Lock()
	defer user.rosterMu.Unlock()

	user.roster = make(Roster)
	for _, item := range query.Items {
		if item.Subscription == "remove" {
			continue
		}
		user.roster[item.JID] = RosterItem{
			Name:   item.Name,
			Groups: item.Groups,
		}
	}

	return nil
}

func replaceRoster(user *rosterUser, newRoster Roster) ([]xmpp.RosterItem, error) {
	user.rosterMu.Lock()
	defer user.rosterMu.Unlock()
	if user.roster == nil {
		return nil, ErrRosterNotIntialized
	}

	changes := []xmpp.RosterItem{}

	for jid, newItem := range newRoster {
		curItem, exists := user.roster[jid]
		if !exists || !curItem.Equal(newItem) {
			user.roster[jid] = newItem
			changes = append(changes, xmpp.RosterItem{
				JID:          jid,
				Name:         newItem.Name,
				Subscription: "both",
				Groups:       newItem.Groups,
			})
		}
	}
	for jid := range user.roster {
		_, exists := newRoster[jid]
		if !exists {
			delete(user.roster, jid)
			changes = append(changes, xmpp.RosterItem{
				JID:          jid,
				Subscription: "remove",
			})
		}
	}

	return changes, nil
}

func (service *Service) SetRoster(ctx context.Context, userJID xmpp.Address, newRoster Roster) error {
	user, userExists := service.rosterUsers[userJID]
	if !userExists {
		return errors.New("no such user")
	}
	return service.setRoster(ctx, userJID, user, newRoster)
}

func (service *Service) setRoster(ctx context.Context, userJID xmpp.Address, user *rosterUser, newRoster Roster) error {
	changes, err := replaceRoster(user, newRoster)
	if err != nil {
		return err
	}

	for _, changedItem := range changes {
		query := xmpp.RosterQuery{
			Items: []xmpp.RosterItem{changedItem},
		}
		if err := service.sendXMPPRosterQuery(xmpp.RandomID(), userJID, "set", query); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) sendXMPPRosterQuery(id string, to xmpp.Address, iqType string, query xmpp.RosterQuery) error {
	iq := &xmpp.Iq{
		Header: xmpp.Header{
			ID: id,
			To: &to,
		},
		Type: iqType,
		RosterQuery: &query,
	}
	if !service.sendWithin(5*time.Second, iq) {
		return errors.New("Timed out when sending XMPP iq message")
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
	if !service.sendWithin(5*time.Second, xmppMessage) {
		return errors.New("Timed out when sending XMPP error message")
	}
	return nil
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
	if !service.sendWithin(5*time.Second, xmppPresence) {
		return errors.New("Timed out when sending XMPP presence")
	}
	return nil
}

func (service *Service) addressForPhoneNumber(phoneNumber string) (xmpp.Address, bool) {
	for address, user := range service.users {
		if user.phoneNumber == phoneNumber {
			return address, true
		}
	}
	return xmpp.Address{}, false
}

func (service *Service) canonPhoneNumber(phoneNumber string) (string, error) {
	if !strings.HasPrefix(phoneNumber, "+") {
		if service.defaultPrefix == "" {
			return "", errors.New("does not start with + (please prefix number with + and a country code, or configure the default_prefix option)")
		}
		phoneNumber = service.defaultPrefix + phoneNumber
	}
	return phoneNumber, validatePhoneNumber(phoneNumber)
}

func (service *Service) friendlyPhoneNumber(phoneNumber string) string {
	return strings.TrimPrefix(phoneNumber, service.defaultPrefix)
}
