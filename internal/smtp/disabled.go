package smtp

import "errors"

// ErrDisabled indicates that outbound email is disabled for this process.
var ErrDisabled = errors.New("outbound email is disabled in personal mode")

// DisabledMailer prevents outbound email without constructing an SMTP client.
type DisabledMailer struct{}

func (DisabledMailer) Send(_, _, _, _ string) error { return ErrDisabled }

func (DisabledMailer) SendFromTemplate(_ string, _ any, _ ...string) error {
	return ErrDisabled
}
