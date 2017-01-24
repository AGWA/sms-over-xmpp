package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sheenobu/go-xco"
)

type StaticConfig struct {
	Xmpp StaticConfigXmpp `toml:"xmpp"`
}

type StaticConfigXmpp struct {
	Host   string `toml:"host"`
	Name   string `toml:"name"`
	Port   int    `toml:"port"`
	Secret string `toml:"secret"`
}

func main() {
	config := new(StaticConfig)
	_, err := toml.DecodeFile(os.Args[1], &config)
	if err != nil {
		panic(err)
	}

	opts := xco.Options{
		Name:         config.Xmpp.Name,
		SharedSecret: config.Xmpp.Secret,
		Address:      fmt.Sprintf("%s:%d", config.Xmpp.Host, config.Xmpp.Port),
	}
	c, err := xco.NewComponent(opts)
	if err != nil {
		panic(err)
	}

	// Uppercase Echo Component
	c.MessageHandler = xco.BodyResponseHandler(func(msg *xco.Message) (string, error) {
		return strings.ToUpper(msg.Body), nil
	})

	c.Run()
}
