package smsxmpp

import (
	"github.com/emersion/go-webdav/carddav"
	"github.com/emersion/go-vcard"
	"src.agwa.name/go-xmpp"
)

type addressBook map[string]*carddav.AddressObject

func (addrbook addressBook) makeRoster(domain string) Roster {
	roster := make(Roster)
	for _, object := range addrbook {
		name := object.Card.PreferredValue(vcard.FieldFormattedName)
		cellNumber := getVcardCellNumber(object.Card)
		if name != "" && cellNumber != "" {
			// TODO: deterministically handle the case where we have two vcards with the same phone number
			jid := xmpp.Address{LocalPart: cellNumber, DomainPart: domain}
			roster[jid] = RosterItem{
				Name: name,
			}
		}
	}
	return roster
}

func getVcardCellNumber(card vcard.Card) string {
	for _, field := range card[vcard.FieldTelephone] {
		if field.Params.HasType(vcard.TypeCell) {
			return cleanupVcardPhoneNumber(field.Value)
		}
	}
	return ""
}

func cleanupVcardPhoneNumber(num string) string {
	ret := "+"
	for _, c := range num {
		if c >= '0' && c <= '9' {
			ret += string(c)
		}
	}
	return ret
}
