package sms // import "github.com/mndrix/sms-over-xmpp"
import xco "github.com/mndrix/go-xco"

type StaticConfig struct {
	Xmpp StaticConfigXmpp `toml:"xmpp"`

	// Users maps the local part of an XMPP address to the
	// corresponding E.164 phone number.
	Users map[string]string `toml:"users"`
}

type StaticConfigXmpp struct {
	Host   string `toml:"host"`
	Name   string `toml:"name"`
	Port   int    `toml:"port"`
	Secret string `toml:"secret"`
}

func (self *StaticConfig) ComponentName() string {
	return self.Xmpp.Name
}

func (self *StaticConfig) SharedSecret() string {
	return self.Xmpp.Secret
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
