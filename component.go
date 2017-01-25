package sms // import "github.com/mndrix/sms-over-xmpp"

import (
	"fmt"
	"strings"

	"github.com/sheenobu/go-xco"
)

func Main(config Config) {
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
