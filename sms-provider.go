package sms // import "github.com/mndrix/sms-over-xmpp"

// SmsProvider describes a provider that is able to send and receive
// SMS messages.
type SmsProvider interface {
	// SendSms sends an SMS to the given recipient with the given
	// caller ID.
	SendSms(from, to, body string) error
}
