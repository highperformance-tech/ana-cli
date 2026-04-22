# ana-cli

`ana` is a command-line client for [TextQL](https://app.textql.com). It speaks
the Connect-RPC endpoints that power the TextQL web app and exposes them as a
scriptable CLI for automation, CI pipelines, and power-user workflows.

## Installation

### Install script (linux / macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/highperformance-tech/ana-cli/main/install.sh | sh
```

Installs the latest release into `/usr/local/bin/ana`. Override with
`INSTALL_DIR=$HOME/.local/bin`.

### `go install`

```bash
go install github.com/highperformance-tech/ana-cli/cmd/ana@latest
```

### Download a release archive

Grab the matching `ana_<version>_<os>_<arch>.tar.gz` (or `.zip` on Windows)
from the [releases page](https://github.com/highperformance-tech/ana-cli/releases),
extract the `ana` binary, and drop it on your `PATH`. Each release ships a
`checksums.txt` you can verify with `sha256sum -c checksums.txt`.

### Build from source

```bash
git clone https://github.com/highperformance-tech/ana-cli.git
cd ana-cli
make build
./bin/ana --version
```

Windows users: see [docs/windows-smartscreen.md](docs/windows-smartscreen.md).

## Usage

```bash
ana [global flags] <command> [args]
```

```bash
ana auth login --endpoint https://app.textql.com
ana org show
ana connector list
ana chat send "show me last month's revenue"
ana update  # replace the running binary with the latest release
```

Run `ana --help` or `ana <verb> --help` for command-specific flags.

`ana` checks GitHub for a newer release after each verb and prints a one-line
stderr nudge when one exists. The result is cached for 4 h by default; set
`updateCheckInterval` in `config.json` (any `time.ParseDuration`-compatible
value) to change the cadence, or `"0"` / `"disable"` to turn the check off.
`--json` suppresses the nudge so automation pipelines aren't broken.

## Configuration

`ana` stores tokens and per-profile endpoints at
`$XDG_CONFIG_HOME/ana/config.json` (falling back to `~/.config/ana/config.json`).

## Development

```bash
make test          # go test -race ./...
make cover         # enforces 100% coverage on internal/...
make lint          # gofmt, go vet, staticcheck
make build         # -> ./bin/ana
make release-local # goreleaser check + snapshot (requires goreleaser)
```

## License

MIT — see [LICENSE](LICENSE).
