package main

type StaticConfig struct {
	Xmpp StaticConfigXmpp `toml:"xmpp"`
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
