package main

import (
	"strings"

	"github.com/sheenobu/go-xco"
)

func main() {
	opts := xco.Options{
		Name:         "sms.example.com",
		SharedSecret: "secret shared with the XMPP server",
		Address:      "127.0.0.1:5347",
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
