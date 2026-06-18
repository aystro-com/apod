package engine

import (
	"errors"
	"fmt"
)

// ErrorKind classifies an engine error so the API layer can map it to the right
// HTTP status instead of a blanket 500. The zero value (KindInternal) is for
// genuine server-side failures.
type ErrorKind int

const (
	KindInternal  ErrorKind = iota // 500 — unexpected server-side failure
	KindInvalid                    // 400 — bad/invalid client input
	KindNotFound                   // 404 — referenced resource does not exist
	KindConflict                   // 409 — resource already exists / state conflict
	KindForbidden                  // 403 — not permitted
)

// Error is a typed engine error carrying an ErrorKind. It wraps an optional
// underlying error so errors.Is/As keep working through it.
type Error struct {
	Kind ErrorKind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

// Invalid, NotFound, Conflict and Forbidden construct typed client errors.
func Invalid(format string, a ...any) error {
	return &Error{Kind: KindInvalid, Msg: fmt.Sprintf(format, a...)}
}

func NotFound(format string, a ...any) error {
	return &Error{Kind: KindNotFound, Msg: fmt.Sprintf(format, a...)}
}

func Conflict(format string, a ...any) error {
	return &Error{Kind: KindConflict, Msg: fmt.Sprintf(format, a...)}
}

func Forbidden(format string, a ...any) error {
	return &Error{Kind: KindForbidden, Msg: fmt.Sprintf(format, a...)}
}

// WrapInvalid tags an existing error as invalid client input, preserving it for
// errors.Is/As.
func WrapInvalid(err error, msg string) error {
	return &Error{Kind: KindInvalid, Msg: msg, Err: err}
}

// ErrorKindOf reports the kind of any error, returning KindInternal for errors
// that are not (and do not wrap) a typed engine Error.
func ErrorKindOf(err error) ErrorKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindInternal
}
