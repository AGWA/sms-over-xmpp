/*
 * Copyright (c) 2019 Andrew Ayer
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * Except as contained in this notice, the name(s) of the above copyright
 * holders shall not be used in advertising or otherwise to promote the
 * sale, use or other dealings in this Software without prior written
 * authorization.
 */

package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"

	"src.agwa.name/go-listener"
	_ "src.agwa.name/go-listener/tls"
	"src.agwa.name/sms-over-xmpp"
	"src.agwa.name/sms-over-xmpp/config"
	_ "src.agwa.name/sms-over-xmpp/providers/nexmo"
	_ "src.agwa.name/sms-over-xmpp/providers/twilio"
)

func main() {
	var flags struct {
		config string
		listen []string
	}
	flag.StringVar(&flags.config, "config", "", "Path to config directory")
	flag.Func("listen", "Socket to listen on (repeatable)", func(arg string) error {
		flags.listen = append(flags.listen, arg)
		return nil
	})
	flag.Parse()

	if flags.config == "" {
		log.Fatal("-config flag not specified")
	}
	if len(flags.listen) == 0 {
		log.Fatal("-listen flag not specified")
	}

	config, err := config.FromDirectory(flags.config)
	if err != nil {
		log.Fatal(err)
	}

	listeners, err := listener.OpenAll(flags.listen)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.CloseAll(listeners)

	service, err := smsxmpp.NewService(config)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := http.Server{
		Handler: service.HTTPHandler(),
	}

	for _, l := range listeners {
		go func(l net.Listener) {
			log.Fatal(httpServer.Serve(l))
		}(l)
	}

	go func() {
		if err := service.RunAddressBookUpdater(context.Background()); err != nil {
			log.Fatal(err)
		}
	}()

	log.Fatal(service.RunXMPPComponent(context.Background()))
}
