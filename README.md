# SMS-over-XMPP

sms-over-xmpp is an XMPP component (XEP-0114) that acts as a gateway
between an XMPP (aka Jabber) network and the SMS network.  It allows
you to send and receive SMS messages as if they were XMPP messages,
using your favorite XMPP client.

You send an XMPP message and your friend receives an SMS.  When they
send you an SMS, you receive an XMPP message.

## Supported SMS Providers

* Twilio (recommended)
* Nexmo/Vonage
* Signalwire

## Features

### MMS

Sending MMS requires your XMPP client/server to support:

* XEP-0363 (HTTP File Upload)
* XEP-0066 (Out of Band Data)

Receiving MMS requires your client to support:

* XEP-0066 (Out of Band Data)

If your client does not support XEP-0066, then incoming MMS will
contain a URL to the media file.

### CardDAV Roster Synchronization

sms-over-xmpp can optionally synchronize a CardDAV address book with your
XMPP roster.  Any contact in your address book with a mobile phone number
is added to your XMPP roster with the necessary address to send them
an SMS.  When a contact is deleted, it is also deleted from your roster.

The synchronization is one-way: changes made to your XMPP roster aren't
propagated to your address book, and may be reverted.

Your XMPP server must support
[XEP-0321](https://xmpp.org/extensions/xep-0321.html).  For Prosody,
you can use [mod_remote_roster](contrib/mod_remote_roster.lua).

## Documentation

* [Config Reference](doc/configuration.md)

## Installation

```
go install src.agwa.name/sms-over-xmpp/cmd/sms-over-xmpp@latest
```

## Tested Configurations

sms-over-xmpp has been tested with the following configuration:

* [Twilio](https://www.twilio.com/) for the SMS provider
* [Prosody](https://prosody.im/) for the XMPP server
* [Gajim](https://gajim.org/) for the desktop XMPP client
* [Monal](https://monal.im/) for the mobile (iOS) XMPP client
* [Radicale](https://radicale.org/) for the CardDAV server (needed for address book synchronization)
* [mod_http_upload_s3](https://github.com/abeluck/mod_http_upload_s3) for HTTP file upload in Prosody (needed for sending MMS)
* [mod_remote_roster.lua](contrib/mod_remote_roster.lua) for remote roster management in Prosody (needed for address book synchronization)
