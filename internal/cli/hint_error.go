package cli

import "errors"

// hintError is an error that carries a user-facing hint alongside the primary message.
// The hint is printed separately by ExitErrHandler so it does not pollute the error string.
type hintError struct {
	err  error
	hint string
}

func (e *hintError) Error() string { return e.err.Error() }
func (e *hintError) Unwrap() error { return e.err }

// withHint wraps err and attaches a hint that is displayed after the error message.
func withHint(err error, hint string) error {
	return &hintError{err: err, hint: hint}
}

// newHintError creates a fresh error with the given message and hint.
func newHintError(msg, hint string) error {
	return &hintError{err: errors.New(msg), hint: hint}
}
