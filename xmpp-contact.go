package sms

type pendingBool int

const (
	no      pendingBool = 0
	yes     pendingBool = 1
	pending pendingBool = 2
)

// xmppContact represents a remote XMPP user with whom a local XMPP
// user has contact.
type xmppContact struct {
	// localName is the name by which the local user is known to the
	// remote user.  If empty, no name is known.
	localName string

	// subTo indicates whether the local user has a presence
	// subscription to a remote user.  That is, does the remote user
	// send us his presence updates?
	subTo pendingBool

	// subFrom indicates whether a local user has a presence
	// subscription from a remote user.  That is, do we send our
	// presence updates to the remote user?
	subFrom pendingBool
}
