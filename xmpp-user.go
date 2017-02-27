package sms

import xco "github.com/mndrix/go-xco"

// xmppUser represents a XMPP local user.  That is, a phone number on
// the PSTN who we simulate as having a local XMPP account.
type xmppUser struct {
	// roster records the XMPP users with whom this user has contact.
	roster map[xco.Address]*xmppContact
}

// contact returns the XMPP contact for a remote JID.  creates an
// empty contact if none exists.
func (u *xmppUser) contact(a *xco.Address) *xmppContact {
	remote := *(a.Bare())

	if u.roster == nil {
		u.roster = make(map[xco.Address]*xmppContact)
	}
	contact, ok := u.roster[remote]
	if !ok {
		contact = &xmppContact{}
		u.roster[remote] = contact
	}
	return contact
}
