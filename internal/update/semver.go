package update

import (
	"strconv"
	"strings"
)

// CmpSemver returns -1/0/+1 comparing two three-int semver strings. Accepts
// optional leading `v` and an optional `-prerelease` suffix. Prerelease
// versions sort below their non-prerelease counterparts at the same X.Y.Z
// (so goreleaser's `prerelease: auto` beta→stable path notifies correctly).
// Malformed input on either side returns 0 — a bad tag must never trigger a
// nudge.
func CmpSemver(a, b string) int {
	av, aPre, aOK := parseSemver(a)
	bv, bPre, bOK := parseSemver(b)
	if !aOK || !bOK {
		return 0
	}
	for i := range 3 {
		if av[i] < bv[i] {
			return -1
		}
		if av[i] > bv[i] {
			return 1
		}
	}
	// X.Y.Z match: prerelease < release at the same core version.
	if aPre == "" && bPre != "" {
		return 1
	}
	if aPre != "" && bPre == "" {
		return -1
	}
	if aPre < bPre {
		return -1
	}
	if aPre > bPre {
		return 1
	}
	return 0
}

// parseSemver splits s into (major, minor, patch), a prerelease string (may
// be ""), and an ok flag. Rejects any component that isn't a non-negative
// int. Build metadata after `+` is stripped per semver.
func parseSemver(s string) ([3]int, string, bool) {
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, "", false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, "", false
		}
		out[i] = n
	}
	return out, pre, true
}
