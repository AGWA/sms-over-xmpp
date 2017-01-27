package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"errors"

	xco "github.com/mndrix/go-xco"
)

// StaticConfig intends to implement the Config interface
var _ Config = &StaticConfig{}

type StaticConfig struct {
	Http HttpConfig `toml:"http"`

	Xmpp StaticConfigXmpp `toml:"xmpp"`

	// Users maps the local part of an XMPP address to the
	// corresponding E.164 phone number.
	Users map[string]string `toml:"users"`

	// Twilio contains optional account details for making API calls via the
	// Twilio service.
	Twilio *TwilioConfig `toml:"twilio"`
}

type HttpConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type StaticConfigXmpp struct {
	Host   string `toml:"host"`
	Name   string `toml:"name"`
	Port   int    `toml:"port"`
	Secret string `toml:"secret"`
}

type TwilioConfig struct {
	AccountSid string `toml:"sid"`
	AuthToken  string `toml:"token"`
}

func (self *StaticConfig) ComponentName() string {
	return self.Xmpp.Name
}

func (self *StaticConfig) SharedSecret() string {
	return self.Xmpp.Secret
}

func (self *StaticConfig) HttpHost() string {
	return self.Http.Host
}

func (self *StaticConfig) HttpPort() int {
	port := self.Http.Port
	if port == 0 {
		port = 9677
	}
	return port
}

func (self *StaticConfig) XmppHost() string {
	return self.Xmpp.Host
}

func (self *StaticConfig) XmppPort() int {
	return self.Xmpp.Port
}

func (self *StaticConfig) AddressToPhone(addr xco.Address) (string, error) {
	e164, ok := self.Users[addr.LocalPart]
	if ok {
		return e164, nil
	}

	return addr.LocalPart, nil
}

func (self *StaticConfig) SmsProvider(from, to string) (SmsProvider, error) {
	if self.Twilio == nil {
		return nil, errors.New("Need to configure an SMS provider")
	}
	twilio := &Twilio{
		accountSid: self.Twilio.AccountSid,
		authToken:  self.Twilio.AuthToken,
	}
	return twilio, nil
}
