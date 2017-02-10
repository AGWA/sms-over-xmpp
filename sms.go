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
// rxSms represents information we've received about an SMS. it could
// be a new message arriving or a status update about a message we
// sent.
type rxSms interface {
	// IsRxSms is a dummy method for tagging those types which
	// represent incoming SMS data.
	IsRxSms()

	// ErrCh returns a channel on which to report errors that happen
	// while processing this SMS.
	ErrCh() chan<- error
}
