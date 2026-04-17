package transport

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorString(t *testing.T) {
	e := &Error{HTTPStatus: 418, Code: "teapot", Message: "short and stout"}
	got := e.Error()
	want := "transport: 418 teapot: short and stout"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestIsAuth(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"401 status", &Error{HTTPStatus: 401}, true},
		{"unauthenticated code", &Error{Code: "unauthenticated"}, true},
		{"other code", &Error{Code: "invalid_argument", HTTPStatus: 400}, false},
		{"wrapped 401", fmt.Errorf("outer: %w", &Error{HTTPStatus: 401}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAuth(tc.err); got != tc.want {
				t.Fatalf("IsAuth(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
