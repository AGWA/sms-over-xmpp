package main

// Config describes the minimum methods necessary for configuring an
// sms-over-xmpp component.  These are methods for which no sensible
// default is possible.  Optional configuration methods are described
// by other interfaces.
type Config interface {
	// ComponentName is a name (usually a domain name) by which the
	// XMPP server knows us.
	ComponentName() string

	// SharedSecret is the secret with which we can authenticate to
	// the XMPP server.
	SharedSecret() string

	// XmppHost is the domain name or IP address of the XMPP server.
	XmppHost() string

	// XmppPort is the port on which the XMPP server is listening.
	XmppPort() int
}
