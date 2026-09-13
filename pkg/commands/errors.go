package commands

import (
	"fmt"
	"net/url"

	"github.com/go-errors/errors"
)

// WrapError wraps an error for the sake of showing a stack trace at the top level
func WrapError(err error) error {
	if err == nil {
		return err
	}

	return errors.Wrap(err, 0)
}

// IsConnectionError reports whether a request never reached the daemon. The
// client returns *url.Error for those and api.StatusError for anything the
// daemon answered, so the type is the whole test.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}

	var urlErr *url.Error

	return errors.As(err, &urlErr)
}

// ConnectError is a daemon we never got as far as talking to. Nothing runs
// without a client, so this ends the program with a message.
type ConnectError struct {
	Remote string
	Err    error
}

func (e *ConnectError) Error() string {
	if e.Remote == "" {
		return e.Err.Error()
	}

	return fmt.Sprintf("remote %q: %v", e.Remote, e.Err)
}

func (e *ConnectError) Unwrap() error {
	return e.Err
}

// IsConnectError covers both: never connected, or dropped out since.
func IsConnectError(err error) bool {
	var connectErr *ConnectError

	return errors.As(err, &connectErr) || IsConnectionError(err)
}
