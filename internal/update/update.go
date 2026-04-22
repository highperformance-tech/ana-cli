// Package update powers the passive update nudge (CachedCheck) and the
// `ana update` self-update verb (SelfUpdate). All logic sits behind
// injected deps so cmd/ana stays thin wiring.
//
// Stdlib-only; mirrors goreleaser's archive layout and install.sh's URL
// template.
package update

import "net/http"

// HTTPDoer is the narrow HTTP interface this package consumes. http.Client
// satisfies it; tests pass a fake that returns canned responses.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Repository coordinates. Package vars (not constants) so tests can repoint
// them at a httptest.Server. Callers must not mutate these at runtime.
var (
	latestReleaseURL = "https://api.github.com/repos/highperformance-tech/ana-cli/releases/latest"
	releasesBaseURL  = "https://github.com/highperformance-tech/ana-cli/releases/download"
)
