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

type smsStatus byte

const (
	// smsDelivered means that an SMS message has been delivered to its
	// final destination.
	smsDelivered smsStatus = 1
)

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

// rxSmsMessage represents a newly arrived message
type rxSmsMessage struct {
	// sms is the message content
	sms *Sms

	// errCh is the channel for implement ErrCh() method
	errCh chan<- error
}

// implement rxSms interface
var _ rxSms = &rxSmsMessage{}

func (*rxSmsMessage) IsRxSms()              {}
func (i *rxSmsMessage) ErrCh() chan<- error { return i.errCh }

// rxSmsStatus represents a status update for a message we sent.
type rxSmsStatus struct {
	// id identifies the SMS message to which this status applies.
	id string

	// status is the status of the message
	status smsStatus

	// errCh is the channel for implement ErrCh() method
	errCh chan<- error
}

// implement rxSms interface
var _ rxSms = &rxSmsStatus{}

func (*rxSmsStatus) IsRxSms()              {}
func (i *rxSmsStatus) ErrCh() chan<- error { return i.errCh }
