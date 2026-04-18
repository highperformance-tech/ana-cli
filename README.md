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

### Windows SmartScreen

The Windows binary is unsigned. On first run SmartScreen may block it with
"Windows protected your PC". Click **More info → Run anyway**. To avoid the
prompt altogether, unblock the executable before running:

```powershell
Unblock-File -Path .\ana.exe
```

## Usage

```bash
ana [global flags] <command> [args]
```

Global flags:

| Flag | Description |
|------|-------------|
| `--endpoint <url>` | Override the API endpoint |
| `--token-file <path>` | Path to a bearer-token file |
| `--profile <name>` | Select a config profile |
| `--json` | Emit JSON output |
| `--verbose`, `-v` | Verbose logging |
| `--version`, `-V` | Print version info |

### Getting started

```bash
ana auth login --endpoint https://app.textql.com
ana org show
ana connector list
ana chat send "show me last month's revenue"
```

Run `ana --help` or `ana <verb> --help` for command-specific flags.

## Configuration

`ana` stores tokens and per-profile endpoints at
`$XDG_CONFIG_HOME/ana/config.json` (falling back to
`~/.config/ana/config.json`). Override with `--token-file` or the
`ANA_TOKEN_FILE` environment variable.

## Development

```bash
make test          # go test -race ./...
make cover         # enforces 100% coverage on internal/...
make lint          # gofmt, go vet, staticcheck
make build         # -> ./bin/ana
make release-local # goreleaser check + snapshot (requires goreleaser)
```

Conventional commits drive the release pipeline: a `feat:` or `fix:` landing
on `main` causes [release-please](https://github.com/googleapis/release-please)
to open a PR; merging that PR tags the release and triggers GoReleaser to
publish binaries, archives, checksums, and SBOMs to GitHub Releases.

## License

MIT — see [LICENSE](LICENSE).
