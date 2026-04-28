# internal/update

Powers the passive "new release available" nudge and the `ana update` self-update verb. Stdlib-only by design (the module's zero-deps posture is enforced in review); mirrors the archive layout from `.goreleaser.yml` and the URL template from `install.sh`. Split across small files so each concern (semver compare, nudge check, self-update orchestration, HTTP download, archive extraction) can be read on its own.

## Files

- `update.go` — package doc, `HTTPDoer`, and the URL package-vars tests repoint at `httptest` servers.
- `semver.go` — `CmpSemver` + `parseSemver`. Prerelease sorts below release at the same X.Y.Z; malformed input returns 0 so junk never triggers a nudge.
- `check.go` — passive nudge surface: `CachePath` (XDG → HOME), `ParseInterval` (`nil`→4h, `"0"`/`"disable"`→off), `CachedCheck` (atomic `update-check.json` rw).
- `selfupdate.go` — `Deps` + `SelfUpdate` orchestration (resolve → compare → download → verify → extract → atomic replace), plus `atomicReplace` (Unix rename-over; Windows rename-aside + rollback) and `formatReplaceErr` (EACCES → platform-aware sudo/Admin guidance).
- `download.go` — `httpGet`, `downloadFile` (streams + sha256 via `TeeReader`), `downloadBody` (1 MiB cap for checksums.txt), `verifyChecksum` (parses both goreleaser and sha256sum line formats).
- `extract.go` — `extractBinary` dispatches on `archiveExt` to the tar.gz/zip walker, ending in `writeBinary` (0755).
- `<source>_test.go` — one per source; shared helpers (`fakeDoer`, `releaseServer`, etc.) live in `update_test.go`. 100% coverage gate.
