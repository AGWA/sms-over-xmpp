# SMS over XMPP

sms-over-xmpp is an XMPP component (XEP-0114) that acts as a gateway
between an XMPP network and the SMS network.  It allows you to send
and receive SMS messages as if they were XMPP messages, using your
favorite XMPP client.

You send an XMPP message and your friend receives an SMS.  When she
responds by SMS, you receive an XMPP message.

# Prerequisites

You'll need the following available to install and run sms-over-xmpp.

  * [Go](https://golang.org/dl/)
  * An XMPP server (I like [Prosody](http://prosody.im/))
  * A [Twilio account](https://www.twilio.com/)
  * Ability to receive incoming HTTP requests

## Twilio account

You need a [Twilio account](https://www.twilio.com/) to interact with
the phone network.  Eventually I'd like to add support for other
telephony providers, but Twilio is all that's supported for now.

After creating your
account,
[create an API Key](https://www.twilio.com/console/sms/dev-tools/api-keys).
Make note of the Key SID and Key Token for later in the installation.
This key allows sms-over-xmpp to send outgoing SMS on your behalf.

To send and receive SMS, you'll also need
a
[Twilio phone number](https://www.twilio.com/console/phone-numbers/incoming).
Unfortunately, a verified
number
[cannot send SMS](https://support.twilio.com/hc/en-us/articles/223135427-What-s-the-difference-between-a-verified-phone-number-and-a-Twilio-phone-number-).
It must be a Twilio number.

## XMPP server

sms-over-xmpp is an XMPP component as defined
in [XEP-0114](http://xmpp.org/extensions/xep-0114.html).  That means
that it needs an existing XMPP server to interact with the XMPP
network.  There are several open source XMPP servers available.  My
favorite is [Prosody](http://prosody.im/).  It's easy to configure and
operate.  [ejabberd](https://www.ejabberd.im/) is another popular
choice.

Once your XMPP server is running, you'll need to add an external
component for sms-over-xmpp.  Instructions are available:

  * for [Prosody](http://prosody.im/doc/components#adding_an_external_component)
  * for [ejabberd](https://www.ejabberd.im/node/5134)

You'll need to enter the component's host name and password in your
sms-over-xmpp configuration file later.

## Receive HTTP requests

If you want to receive incoming SMS, the server on which sms-over-xmpp
runs must be able to receive incoming HTTP requests.  This is how your
telephony provider delivers messages to sms-over-xmpp.  Ideally you
should support TLS so that your incoming SMS are kept private in
transit.

Configuration details can vary greatly depending on your local
circumstances.  At a minimum, you can open a high numbered port on
your firewall so that your telephony provider can make HTTP requests
directly to sms-over-xmpp.

# Installation

Install the binary with

    go get github.com/AGWA/sms-over-xmpp/...

Write a config file (`config.toml` is a common name):

```toml
# define how to connect to your XMPP server
[xmpp]
host = "127.0.0.1"
port = 5347
name = "sms.example.com"
secret = "shared secret from your XMPP server config"

# Twilio API details
[twilio]
account-sid = "AC..."
key-sid = "SK..."
key-secret = "..."

# map XMPP usernames to E.164 phone numbers
[users]
"john@example.com" = "+13075551212"
```

Run your SMS component:

    sms-over-xmpp config.toml

To receive incoming SMS, you also have to associate each of your
Twilio phone numbers with a messaging URL.  It's usually easiest to
configure
a
[TwiML App](https://www.twilio.com/console/sms/dev-tools/twiml-apps/add) then
attach it to each of your phone numbers.  The messaging URL is the
address through which Twilio can contact sms-over-xmpp with an HTTP
request.
