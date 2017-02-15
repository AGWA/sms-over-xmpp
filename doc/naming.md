# Naming Conventions

## Tx and Rx

In telecommunications, "tx" is common parlance for "transmit" or
"transmission" while "rx" is for "receive" or "reception".  We follow
that convention within the code.  Tx is for Go channels which carry
messages outward from sms-over-xmpp regardless of the direction of the
Go channel.  Rx is for Go channels carrying messages inward towards
sms-over-xmpp.
