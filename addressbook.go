package smsxmpp

import (
	"context"
	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"golang.org/x/sync/errgroup"
	"src.agwa.name/go-xmpp"
	"strings"

	"log"
)

type addressBook struct {
	changed   bool
	syncToken string
	entries   map[string]carddav.AddressObject // map from path -> object
}

func (addrbook *addressBook) download(ctx context.Context, client *carddav.Client) error {
	log.Printf("Sync token = %q", addrbook.syncToken)
	response, err := client.SyncCollection("", &carddav.SyncQuery{
		DataRequest: carddav.AddressDataRequest{AllProp: true},
		SyncToken:   addrbook.syncToken,
	})
	if err != nil {
		return err
	}
	if err := addrbook.downloadObjects(ctx, client, response.Updated); err != nil {
		return err
	}

	if addrbook.entries == nil {
		addrbook.entries = make(map[string]carddav.AddressObject)
		addrbook.changed = true
	}
	for _, updatedObject := range response.Updated {
		log.Printf("Adding %#v to address book", updatedObject)
		addrbook.entries[updatedObject.Path] = updatedObject
		addrbook.changed = true
	}
	for _, deletedPath := range response.Deleted {
		log.Printf("Deleting %s from address book", deletedPath)
		delete(addrbook.entries, deletedPath)
		addrbook.changed = true
	}
	addrbook.syncToken = response.SyncToken
	return nil
}

func (addrbook *addressBook) downloadObjects(ctx context.Context, client *carddav.Client, objects []carddav.AddressObject) error {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(10)
	for i := range objects {
		i := i
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			object, err := client.GetAddressObject(objects[i].Path)
			if err != nil {
				return err
			}
			objects[i] = *object
			return nil
		})
	}
	return group.Wait()
}

func (addrbook *addressBook) makeRoster(domain string) Roster {
	roster := make(Roster)
	for _, object := range addrbook.entries {
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
	num = strings.TrimPrefix(num, "011")
	ret := "+"
	for _, c := range num {
		if c >= '0' && c <= '9' {
			ret += string(c)
		}
	}
	return ret
}
