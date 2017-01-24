package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sheenobu/go-xco"
)

func main() {
	var config Config
	config = new(StaticConfig)
	_, err := toml.DecodeFile(os.Args[1], &config)
	if err != nil {
		panic(err)
	}

	opts := xco.Options{
		Name:         config.ComponentName(),
		SharedSecret: config.SharedSecret(),
		Address:      fmt.Sprintf("%s:%d", config.XmppHost(), config.XmppPort()),
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
