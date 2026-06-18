package engine

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorKindOf(t *testing.T) {
	cases := []struct {
		err  error
		want ErrorKind
	}{
		{nil, KindInternal},
		{errors.New("boom"), KindInternal},
		{fmt.Errorf("db: %w", errors.New("x")), KindInternal},
		{Invalid("bad %s", "input"), KindInvalid},
		{NotFound("site %q not found", "a.com"), KindNotFound},
		{Conflict("already exists"), KindConflict},
		{Forbidden("nope"), KindForbidden},
		// Kind survives wrapping.
		{fmt.Errorf("context: %w", NotFound("missing")), KindNotFound},
		{WrapInvalid(errors.New("parse"), "invalid key"), KindInvalid},
	}
	for i, c := range cases {
		if got := ErrorKindOf(c.err); got != c.want {
			t.Errorf("case %d: ErrorKindOf = %v, want %v", i, got, c.want)
		}
	}
}

func TestTypedErrorMessage(t *testing.T) {
	if got := Invalid("port %d out of range", 70000).Error(); got != "port 70000 out of range" {
		t.Errorf("message = %q", got)
	}
	wrapped := WrapInvalid(errors.New("underlying"), "invalid input")
	if wrapped.Error() != "invalid input: underlying" {
		t.Errorf("wrapped message = %q", wrapped.Error())
	}
	if !errors.Is(wrapped, errors.Unwrap(wrapped)) {
		t.Error("WrapInvalid should preserve the underlying error for errors.Is")
	}
}
