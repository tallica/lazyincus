package commands

import (
	"github.com/go-errors/errors"
)

// WrapError wraps an error for the sake of showing a stack trace at the top level
func WrapError(err error) error {
	if err == nil {
		return err
	}

	return errors.Wrap(err, 0)
}
