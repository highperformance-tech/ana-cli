# internal/update

Powers the passive "new release available" nudge and the `ana update` self-update verb. Stdlib-only by design (the module's zero-deps posture is enforced in review); mirrors the archive layout from `.goreleaser.yml` and the URL template from `install.sh`. Split across small files so each concern (semver compare, nudge check, self-update orchestration, HTTP download, archive extraction) can be read on its own.

## Files

- `update.go` — package doc, `HTTPDoer` interface, and the two package-level URL vars (`latestReleaseURL`, `releasesBaseURL`) that tests repoint at `httptest` servers.
- `semver.go` — `CmpSemver` + `parseSemver`. Three-int semver with optional `v` prefix and `-prerelease` suffix; prerelease sorts below release at the same X.Y.Z so `prerelease: auto` beta→stable flows notify correctly. Malformed input returns 0 (never trigger a nudge on junk).
- `check.go` — passive nudge surface: `LatestRelease`, `CacheDeps`, `CachePath` (XDG → HOME), `ParseInterval` (`nil`→4h, `"0"`/`"disable"`→off), `CachedCheck` (reads/writes `update-check.json` atomically), plus the unexported `cacheFile` / `readCache` / `writeCache` / `shouldNotify`.
- `selfupdate.go` — `Deps` + `DefaultDeps` + `resolveDeps`, the `SelfUpdate` orchestration (resolve → compare → download → verify → extract → atomic replace), the `updateStatus` JSON shape + `emitStatus` (routes JSON through `cli.WriteJSON` for byte-compat), and `atomicReplace` (Unix rename-over; Windows rename-aside + rollback).
- `download.go` — HTTP helpers (`httpGet`, `downloadFile` — streams body to disk and returns the sha256 via `TeeReader`, avoiding a re-read, `downloadBody` — 1 MiB-capped for `checksums.txt`) and `verifyChecksum` (parses goreleaser's `<hex>  <filename>` or sha256sum's `<hex> *<filename>` format and compares against the caller-supplied hex).
- `extract.go` — `extractBinary` dispatches on `archiveExt`; `extractFromTarGz` / `extractFromZip` walk the archive for the matching member and hand the reader to `writeBinary` (0755).
- `<source>_test.go` — one test file per source; shared helpers (`fakeDoer`, `releaseServer`, `stageUpdate`, `wantErr`, `withURLs`, `fakeArchive`) live in `update_test.go` per the repo convention. 100% coverage gate.
