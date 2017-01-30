// sms-over-xmpp is a small XMPP component (XEP-0114) which acts as a
// gateway or proxy between XMPP and SMS.  It allows you to send and
// receive SMS messages as if they were XMPP messages.  This lets you
// interact with the SMS network using your favorite XMPP client.
//
// To run sms-over-xmpp, give it the path to your config file:
//
//     sms-over-xmpp example.toml
//
// It connects to your XMPP server and proxies messages between it and
// the SMS network.
package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	sms "github.com/mndrix/sms-over-xmpp"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: sms-over-xmpp config.toml\n")
		os.Exit(1)
	}

	config := new(sms.StaticConfig)
	_, err := toml.DecodeFile(os.Args[1], &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't read config file '%s': %s\n", os.Args[1], err)
		os.Exit(1)
	}
	sms.Main(config)
}
