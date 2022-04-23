# sms-over-xmpp configuration

sms-over-xmpp is configured by command line arguments in conjunction
with text files in a configuration directory.

## Command line arguments

### `-listen LISTENER` (Mandatory)

Listen on the given address, provided in [go-listener
syntax](https://pkg.go.dev/src.agwa.name/go-listener#readme-listener-syntax).
You can specify the `-listen` flag multiple times to listen on multiple
addresses.

Examples:
* `-listen tcp:8080` to listen on TCP port 8080, all interfaces.
* `-listen tcp:192.0.2.4:8080` to listen on TCP port 8080 on 192.0.2.4.
* `-listen tls:sms.example.com:tcp:443` to listen on TCP port 443 with an automatically-obtained HTTPS certificate for `sms.example.com`.
* `-listen tls:/path/to/certificate.pem:tcp:8443` to listen on TCP port 8443 with an HTTPS certificate from the given file (which must be a single PEM file containing the private key, the certificate, and the intermediate certificate(s)).

### `-config PATH` (Mandatory)

Specifies the path to the configuration directory, which must be organized
as described below.

## Configuration directory

The configuration directory contains these files:

* General configuration, in a file named `config`
* The users map, in a file named `users`
* At least one provider configuration file in a file named `providers/NAME`, where `NAME` identifies the provider and can be anything you want
* (Optional) The rosters map, in a file named `rosters`

### Example directory structure

```
config
users
providers/
  personal
  work
rosters
```

### The config file

General configuration is located in a file named `config`.
Each line in the file consists of a parameter name, whitespace,
and the parameter's value.  Blank lines and lines
starting with `#` are ignored.

The parameters are:

| Parameter     | Description                                                 |
| ------------- | ----------------------------------------------------------- |
| `xmpp_server` | The hostname and _component_ port number of your XMPP server |
| `xmpp_domain` | The domain name of the XMPP component                       |
| `xmpp_secret` | The secret for the XMPP component (chosen by you and shared with XMPP server) |

Example `config` file:

```
xmpp_server xmpp.example.com:5347
xmpp_domain sms.example.com
xmpp_secret 6iLNu1YGNCcZyEqn
```

### The users map

A mapping of XMPP users to phone numbers is located in a file named `users`.
Each line in the file consists of a bare Jabber ID (e.g. `andrew@example.com`),
whitespace, the name of the provider handling this user, a colon, and the
phone number in [E.164 format](https://www.twilio.com/docs/glossary/what-e164) (e.g. `+12125551212`).
Blank lines and lines starting with `#` are ignored.

Each provider name must correspond to a file in the `providers`
sub-directory, as described in the next section.

When an XMPP user listed in the users map sends a message, it will be routed
via the corresponding provider with the given source phone number.

When an SMS arrives that is addressed to a phone number in the users map,
it will be sent to the corresponding XMPP user.

Each XMPP user must map to exactly one phone number, and each phone number
must map to exactly one XMPP user.

Example `users` map:

```
andrew@example.com personal:+12125551212
jon@example.com    personal:+14015551122
sales@example.com  work:+14155551221
```

### The rosters map (optional)

The `rosters` file contains a mapping from XMPP users to CardDAV URLs.
For each entry in this file, sms-over-xmpp will synchronize the address
book at the given URL to the XMPP user's roster.

The XMPP server must support
[XEP-0321](https://xmpp.org/extensions/xep-0321.html).  For Prosody,
you can use [mod_remote_roster](../contrib/mod_remote_roster.lua).

Example `rosters` map:

```
andrew@example.com https://andrew:password123@carddav.example.com/andrew/fbacc20f-99ca-4bde-afce-cc05880bdac0/
jon@example.com    https://jon:qwertyuiop@carddav.example.com/jon/5c9b9975-733f-4374-a0a9-dfdfda2f7ba5/
```

### Provider config

Configuration for an SMS provider is located in the `providers`
sub-directory in a file with a name that you choose to identify the
provider.  Each line in the file consists of a parameter name,
whitespace, and the parameter's value.  Blank lines and lines
starting with `#` are ignored.

A provider config file contains both parameters which are common
to all types of providers, and parameters which are specific to the
type of provider.

#### Common parameters

| Parameter     | Description                                                 |
| ------------- | ----------------------------------------------------------- |
| `type`        | The type of provider: `twilio`, `signalwire`, or `nexmo`.   |

#### Twilio-specific parameters

| Parameter       | Description |
| --------------- | ------------|
| `account_sid`   | The main account identifier listed on your [Twilio Console](https://www.twilio.com/console) |
| `key_sid`       | SID for a [Twilio API key](https://www.twilio.com/console/sms/dev-tools/api-keys), provided by Twilio |
| `key_secret`    | Secret for a [Twilio API key](https://www.twilio.com/console/sms/dev-tools/api-keys), provided by Twilio |
| `http_password` | A password, chosen by you, that Twilio must use when executing the webhook for incoming SMSes |

Note that `key_sid` and `key_secret` are distinct from your Twilio "auth token", which won't work here.

Example config file for a Twilio-type provider:

```
type            twilio
account_sid     AC...
key_sid         SK...
key_secret      ...
http_password   EDIVMA8HLvrZOV5N
```

#### Twilio webhook configuration

You must configure your Twilio account to invoke a webhook when you
receive an incoming SMS.  The URL of the webhook follows this template:

`http://twilio:HTTP_PASSWORD@HOSTNAME:PORT/PROVIDER_NAME/message`

Replace:

* `HTTP_PASSWORD` with the password specified to the `http_password` parameter.
* `HOSTNAME:PORT` with the public hostname and port number of your sms-over-xmpp server.
* `PROVIDER_NAME` with the name of the provider.

Example webhook URL: `http://twilio:EDIVMA8HLvrZOV5N@example.com:8080/personal/message`

Note: if you have placed sms-over-xmpp behind a reverse proxy, be sure to adjust
the URL accordingly.

#### SignalWire-specific parameters

| Parameter       | Description |
| --------------- | ------------|
| `domain`        | The domain of your SignalWire space (e.g. `example.signalwire.com`) |
| `project_id`    | The ID of your SignalWire project |
| `auth_token`    | Your SignalWire authentication token |
| `http_password` | A password, chosen by you, that SignalWire must use when executing the webhook for incoming SMSes |

Example config file for a SignalWire-type provider:

```
type            signalwire
domain          example.signalwire.com
project_id      YourProjectID
auth_token      YoruAuthToken
http_password   EDIVMA8HLvrZOV5N
```

#### SignalWire webhook configuration

You must configure your SignalWire account to invoke a webhook when you
receive an incoming SMS.  The URL of the webhook follows this template:

`http://signalwire:HTTP_PASSWORD@HOSTNAME:PORT/PROVIDER_NAME/message`

Replace:

* `HTTP_PASSWORD` with the password specified to the `http_password` parameter.
* `HOSTNAME:PORT` with the public hostname and port number of your sms-over-xmpp server.
* `PROVIDER_NAME` with the name of the provider.

Example webhook URL: `http://signalwire:EDIVMA8HLvrZOV5N@example.com:8080/personal/message`

Note: if you have placed sms-over-xmpp behind a reverse proxy, be sure to adjust
the URL accordingly.

#### Nexmo-specific parameters

| Parameter       | Description |
| --------------- | ------------|
| `api_key`       | Your Nexmo API key, provided by Nexmo |
| `api_secret`    | Your Nexmo API secret, provided by Nexmo |
| `http_password` | A password, chosen by you, that Nexmo must use when executing the webhook for incoming SMSes |

Example config file for a Nexmo-type provider:

```
type            nexmo
api_key         abcd1234
api_secret      abcdef0123456789
http_password   5VKFT8pByMkO6IG6
```

#### Nexmo webhook configuration

You must configure your Nexmo account to invoke a webhook when you
receive an incoming SMS.  The URL of the webhook follows this template:

`http://nexmo:HTTP_PASSWORD@HOSTNAME:PORT/PROVIDER_NAME/inbound-sms`

Replace:

* `HTTP_PASSWORD` with the password specified to the `http_password` parameter.
* `HOSTNAME:PORT` with the public hostname and port number of your sms-over-xmpp server.
* `PROVIDER_NAME` with the name of the provider.

Example webhook URL: `http://nexmo:5VKFT8pByMkO6IG6@example.com:8080/work/message`

Note: if you have placed sms-over-xmpp behind a reverse proxy, be sure to adjust
the URL accordingly.
