# Architecture

This document describes how sms-over-xmpp is arranged internally.

    +-----------------------------------------------------+
    |                                                     |
    |                     Component                       |
    |                                                     |
    |   +-----------+    +-----------+    +-----------+   |
    |   |           +--> |           | <--+           |   |
    |   |  XMPP     |    |  Gateway  |    |  PSTN     |   |
    |   |  Process  |    |  Process  |    |  Process  |   |
    |   |           | <--+           +--> |           |   |
    |   +-----------+    +-----------+    +-----------+   |
    |                                                     |
    +-----------------------------------------------------+

## Goals

This architecture is slightly more complicated than a monolithic
implementation.  That's done with the following goals in mind:

  * resilience against failure
  * fast processing for all messages
  * support for multiple telephony providers

## Component

The component is the main goroutine that's started when the executable
begins.  It's responsible for making sure that the other processes are
running.  If one of them crashes, it starts a new one to replace it.

The component also maintains state that should persist beyond process
crashes.  This includes things like the channels between processes.
Connecting the processes with Go channels makes sure that messages
between processes aren't dropped if a process is in the middle of
restarting.

## XMPP Process

The XMPP process represents the XMPP network.  It maintains a TCP
connection to an XMPP server.  It reads incoming XMPP stanzas and
responds to them as required by the XMPP protocol.  If an XMPP event
needs to be translated into a telephony event (send an SMS, etc), it
sends a value down the channel to the Gateway process.

When the Gateway process needs to interact with the XMPP network, it
sends a value down the channel to the XMPP process which translates it
into an XMPP stanza.

## PSTN Process

The PSTN process represents the entire telephone network.  It listens
for events from a telephony provider (usually via HTTP).  If those
events need to be translated into XMPP, it sends a value down the
channel to the Gateway process.

When the Gateway process needs to interact with the PSTN, it sends a
value down the appropriate channel.

## Gateway Process

The Gateway process translates between the XMPP and PSTN protocols.
During normal operation, it also owns the user's configuration.  The
config specifies how these translations should be done. If the Gateway
crashes, ownership of the config reverts back to the Component which
lends it to the new Gateway once it's started.
