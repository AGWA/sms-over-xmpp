package sms // import "github.com/mndrix/sms-over-xmpp"
import "net/http"

// SmsProvider describes a provider that is able to send and receive
// SMS messages.
type SmsProvider interface {
	// ReceiveSms retrieves the details of an incoming SMS based on an
	// HTTP request.  The details are, respectively: from number, to
	// number and message body.
	ReceiveSms(r *http.Request) (string, string, string, error)

	// SendSms sends an SMS to the given recipient with the given
	// caller ID.  It returns a unique identifier for the outgoing
	// message.  If possible, the identifier should identify this
	// message in the provider's logs.  If not, a random identifier
	// can be used.
	SendSms(from, to, body string) (string, error)
}
