package auth

import (
	"errors"
	"testing"
)

func TestTranslateErrNil(t *testing.T) {
	t.Parallel()
	if translateErr(nil) != nil {
		t.Errorf("nil should pass through")
	}
}

func TestTranslateErrPlainPassthrough(t *testing.T) {
	t.Parallel()
	in := errors.New("random")
	if got := translateErr(in); got != in {
		t.Errorf("plain error mutated: %v", got)
	}
}

func TestAuthErrUnwrapAndError(t *testing.T) {
	t.Parallel()
	inner := errors.New("deep cause")
	wrapped := &authErr{wrapped: inner}
	if wrapped.Error() != "deep cause" {
		t.Errorf("Error()=%q", wrapped.Error())
	}
	if !errors.Is(wrapped, inner) {
		t.Errorf("errors.Is should find inner")
	}
	if !wrapped.IsAuthError() {
		t.Errorf("IsAuthError should return true")
	}
}
