package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	sms "github.com/mndrix/sms-over-xmpp"
)

func main() {
	config := new(sms.StaticConfig)
	_, err := toml.DecodeFile(os.Args[1], &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't read config file '%s': %s\n", os.Args[1], err)
		os.Exit(1)
	}
	sms.Main(config)
}
