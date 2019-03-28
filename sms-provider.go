package sms // import "github.com/AGWA/sms-over-xmpp"

// SmsProvider describes a provider that is able to send and receive
// SMS messages.
type SmsProvider interface {
	// RunPstnProcess creates a goroutine for receiving SMSes.  It returns a
	// channel for monitoring the goroutine's health.  If that channel
	// closes, the SMS goroutine has died.
	RunPstnProcess(chan<- RxSms) <-chan struct{}

	// SendSms sends the given SMS. It returns a unique identifier for
	// the outgoing message.  If possible, the identifier should
	// identify this message in the provider's logs.  If not, a random
	// identifier can be used.
	SendSms(*Sms) (string, error)
}
