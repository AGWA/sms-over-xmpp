package sms // import "github.com/mndrix/sms-over-xmpp"

// Sms represents a single SMS message.
type Sms struct {
	// To is the E.164 phone number to which the message was/will-be
	// sent.
	To string

	// From is the E.164 phone number that send/is-sending the
	// message.
	From string

	// Body is the text content of the message.
	Body string
}
