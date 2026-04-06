package errors

import (
	"errors"
	"fmt"
	"strings"
)

// Code identifies the category of a qvm error.
type Code string

const (
	CodeUnknownVersion   Code = "UNKNOWN_VERSION"
	CodeUnknownArch      Code = "UNKNOWN_ARCH"
	CodeUnknownModule    Code = "UNKNOWN_MODULE"
	CodeNetworkError     Code = "NETWORK_ERROR"
	CodeExtractError     Code = "EXTRACT_ERROR"
	CodeChecksumError    Code = "CHECKSUM_ERROR"
	CodeNotInstalled     Code = "NOT_INSTALLED"
	CodeAlreadyInstalled Code = "ALREADY_INSTALLED"
	CodeNoModules        Code = "NO_MODULES"
	CodeConfigError      Code = "CONFIG_ERROR"
	CodeIOError          Code = "IO_ERROR"
)

// QvmError is the structured error type for qvm.
type QvmError struct {
	Code        Code
	Message     string
	Suggestions []string
	Cause       error
}

// New creates a QvmError with the given code and message.
func New(code Code, message string) *QvmError {
	return &QvmError{Code: code, Message: message}
}

// Newf creates a QvmError with a formatted message.
func Newf(code Code, format string, args ...any) *QvmError {
	return &QvmError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Error implements the error interface, including suggestions if present.
func (e *QvmError) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Message)
	if len(e.Suggestions) > 0 {
		sb.WriteString("\n\nSuggestions:")
		for _, s := range e.Suggestions {
			sb.WriteString("\n  ")
			sb.WriteString(s)
		}
	}
	return sb.String()
}

// Unwrap returns the cause of the error, if any.
func (e *QvmError) Unwrap() error {
	return e.Cause
}

// Wrap wraps a cause error.
func Wrap(code Code, cause error, message string) *QvmError {
	return &QvmError{Code: code, Message: message, Cause: cause}
}

// WithSuggestions returns a copy of the error with additional suggestions.
func (e *QvmError) WithSuggestions(suggestions ...string) *QvmError {
	cp := *e
	cp.Suggestions = make([]string, len(e.Suggestions))
	copy(cp.Suggestions, e.Suggestions)
	cp.Suggestions = append(cp.Suggestions, suggestions...)
	return &cp
}

// IsQvmError reports whether err or any error in its chain is a *QvmError.
func IsQvmError(err error) bool {
	var qe *QvmError
	return errors.As(err, &qe)
}
