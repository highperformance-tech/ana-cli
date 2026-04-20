package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestToken_String_Redacts(t *testing.T) {
	t.Parallel()
	tok := Token("abcdefghij")
	want := RedactToken("abcdefghij")
	if got := tok.String(); got != want {
		t.Errorf("String=%q want %q", got, want)
	}
}

func TestToken_Format_AllVerbsRedact(t *testing.T) {
	t.Parallel()
	tok := Token("abcdefghij")
	raw := "abcdefghij"
	redacted := RedactToken(raw)
	// Every printf verb a caller might reach for must route through Format
	// and land on the redacted form — %q is the tricky one because fmt
	// special-cases string kinds and would otherwise dump the raw bytes.
	verbs := []string{"%s", "%v", "%+v", "%#v", "%q"}
	for _, v := range verbs {
		got := fmt.Sprintf(v, tok)
		if got != redacted {
			t.Errorf("%s=%q want %q", v, got, redacted)
		}
		if strings.Contains(got, raw) {
			t.Errorf("%s leaked raw token: %q", v, got)
		}
	}
}

func TestToken_Value_ReturnsRaw(t *testing.T) {
	t.Parallel()
	tok := Token("abcdefghij")
	if got := tok.Value(); got != "abcdefghij" {
		t.Errorf("Value=%q", got)
	}
}

func TestToken_EmptyRedactsToUnset(t *testing.T) {
	t.Parallel()
	if got := Token("").String(); got != RedactToken("") {
		t.Errorf("empty String=%q", got)
	}
}
