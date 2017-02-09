package sms // import "github.com/mndrix/sms-over-xmpp"
import (
	"net/url"

	xco "github.com/mndrix/go-xco"
	"github.com/pkg/errors"
)

// StaticConfig intends to implement the Config interface
var _ Config = &StaticConfig{}

type StaticConfig struct {
	Http HttpConfig `toml:"http"`

	Xmpp StaticConfigXmpp `toml:"xmpp"`

	// Phones maps an E.164 phone number to an XMPP address.  If a
	// mapping is not found here, the inverse of Users is considered.
	Phones map[string]string `toml:"phones"`

	// Users maps an XMPP address to an E.164 phone number.
	Users map[string]string `toml:"users"`

	// Twilio contains optional account details for making API calls via the
	// Twilio service.
	Twilio *TwilioConfig `toml:"twilio"`
}

type HttpConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`

	Username string `toml:"username"`
	Password string `toml:"password"`

	PublicUrl string `toml:"public-url"`
}

type StaticConfigXmpp struct {
	Host   string `toml:"host"`
	Name   string `toml:"name"`
	Port   int    `toml:"port"`
	Secret string `toml:"secret"`
}

type TwilioConfig struct {
	AccountSid string `toml:"account-sid"`
	KeySid     string `toml:"key-sid"`
	KeySecret  string `toml:"key-secret"`
}

func (self *StaticConfig) ComponentName() string {
	return self.Xmpp.Name
}

func (self *StaticConfig) SharedSecret() string {
	return self.Xmpp.Secret
}

func (self *StaticConfig) HttpHost() string {
	host := self.Http.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return host
}

func (self *StaticConfig) HttpPort() int {
	port := self.Http.Port
	if port == 0 {
		port = 9677
	}
	return port
}

func (self *StaticConfig) HttpUsername() string {
	return self.Http.Username
}

func (self *StaticConfig) HttpPassword() string {
	return self.Http.Password
}

func (self *StaticConfig) XmppHost() string {
	return self.Xmpp.Host
}

func (self *StaticConfig) XmppPort() int {
	return self.Xmpp.Port
}

func (self *StaticConfig) AddressToPhone(addr xco.Address) (string, error) {
	e164, ok := self.Users[addr.LocalPart+"@"+addr.DomainPart]
	if ok {
		return e164, nil
	}

	// assume the name is already a phone number
	return addr.LocalPart, nil
}

func (self *StaticConfig) PhoneToAddress(e164 string) (xco.Address, error) {
	// is there an explicit mapping?
	jid, ok := self.Phones[e164]
	if ok {
		return xco.ParseAddress(jid)
	}

	// maybe there's an implicit mapping
	for jid, phone := range self.Users {
		if phone == e164 {
			return xco.ParseAddress(jid)
		}
	}

	// assume the phone number is the user name
	addr := xco.Address{
		LocalPart:  e164,
		DomainPart: self.Xmpp.Name,
	}
	return addr, nil
}

func (self *StaticConfig) SmsProvider() (SmsProvider, error) {
	if self.Twilio == nil {
		return nil, errors.New("Need to configure an SMS provider")
	}
	twilio := &Twilio{
		accountSid: self.Twilio.AccountSid,
		keySid:     self.Twilio.KeySid,
		keySecret:  self.Twilio.KeySecret,
	}

	// configure public URL for SMS status updates
	if self.Http.PublicUrl != "" {
		u, err := url.Parse(self.Http.PublicUrl)
		if err != nil {
			return nil, errors.Wrap(err, "Invalid public URL")
		}
		if self.Http.Username != "" {
			if self.Http.Password == "" {
				u.User = url.User(self.Http.Username)
			} else {
				u.User = url.UserPassword(self.Http.Username, self.Http.Password)
			}
		}
		twilio.publicUrl = u
	}

	return twilio, nil
}
