package update

import "testing"

func TestCmpSemver(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0}, {"v1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1}, {"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.3.0", -1}, {"2.0.0", "1.9.9", 1}, {"0.0.1", "0.0.2", -1},
		{"1.2.3", "1.2.3+build.5", 0},
		// prerelease < release at same core
		{"1.2.3-beta", "1.2.3", -1}, {"1.2.3", "1.2.3-beta", 1},
		{"1.2.3-alpha", "1.2.3-beta", -1}, {"1.2.3-beta", "1.2.3-alpha", 1},
		{"1.2.3-beta", "1.2.3-beta", 0},
		// malformed → 0
		{"dev", "1.2.3", 0}, {"1.2.3", "dev", 0},
		{"1.2", "1.2.0", 0}, {"-1.0.0", "0.0.1", 0},
		{"1.2.x", "1.2.3", 0}, {"1.-2.3", "1.2.3", 0},
	}
	for _, tc := range cases {
		if got := CmpSemver(tc.a, tc.b); got != tc.want {
			t.Errorf("CmpSemver(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
