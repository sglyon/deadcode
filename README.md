# deadcode

Multi-language dead-code detection orchestrator. Detects languages, runs the
right per-language analyzer, and emits a unified, agent-friendly report.

**Status:** v0.7 — Python (`vulture`), JavaScript/TypeScript (`knip`),
Elixir (`mix compile` + set-theoretic type system), and Go
(`staticcheck` + `golang.org/x/tools/cmd/deadcode`) adapters running
side by side. Unified `.deadcode-ignore.toml` filter, cross-tool
deduplication, Lipgloss `--pretty` and Markdown reporters, Cobra CLI,
cross-platform release binaries via goreleaser.
See [`docs/SPEC.md`](docs/SPEC.md) for the full architecture and roadmap.

## Why

Per-language dead-code tools are excellent (vulture, knip, staticcheck,
cargo-udeps, ...) but each has its own CLI and output format. There is no
unified, agent-friendly multi-language entry point. `deadcode` doesn't
reinvent semantic analysis — it orchestrates the best-in-class tools and
normalizes their output.

For code duplication, use [`jscpd`](https://github.com/kucherenko/jscpd)
instead. `deadcode` is dead-code only.

## Install

Three options, in increasing convenience:

### 1. One-liner install (recommended)

```bash
curl -sSL https://raw.githubusercontent.com/sglyon/deadcode/main/install.sh | sh
```

Detects OS/arch, downloads the latest release binary, and installs to
`/usr/local/bin`. Override with env vars:

```bash
# Specific version
curl -sSL https://raw.githubusercontent.com/sglyon/deadcode/main/install.sh | VERSION=v0.7.0 sh

# Custom install directory (no sudo needed)
curl -sSL https://raw.githubusercontent.com/sglyon/deadcode/main/install.sh | INSTALL_DIR=$HOME/.local/bin sh
```

### 2. `go install` (for Go developers)

```bash
go install github.com/sglyon/deadcode@latest
```

This puts `deadcode` in `$GOPATH/bin` (typically `$HOME/go/bin`).

### 3. Build from source

```bash
git clone https://github.com/sglyon/deadcode
cd deadcode
make build       # produces ./deadcode
make install     # or installs to $GOPATH/bin
```

After installing, run `deadcode doctor` to see which adapters are
usable on your machine. Each adapter wraps an external tool you may
need to install separately — see the table below.

## Build (contributors)

```bash
make build       # local binary
make test        # run tests
make snapshot    # build all release artifacts in dist/ (no publish)
make dogfood     # build + scan deadcode against itself
```

## Usage

```bash
# Default: scan current directory, console output
deadcode

# JSON to a file (agent-friendly)
deadcode scan --json --output /tmp/deadcode.json .

# Markdown report (PR descriptions, Slack, GitHub issues)
deadcode scan --markdown --output report.md .

# Pretty (Lipgloss) output — auto-enabled when stdout is a terminal
deadcode scan .

# Force pretty even when piped
deadcode scan --pretty=always . | less -R

# Disable color (also respects $NO_COLOR)
deadcode scan --no-color .

# Restrict by language and confidence
deadcode scan --lang python --min-confidence 0.8 src/

# CI gate
deadcode scan --threshold 0 --exit-code 1 --json .

# Check which underlying tools are installed
deadcode doctor

# List built-in adapters
deadcode adapters

# Inspect / validate the discovered .deadcode-ignore.toml
deadcode ignore list
deadcode ignore validate

# Show suppressed findings with their ignore reasons
deadcode scan --show-ignored .
```

## Suppressing findings: `.deadcode-ignore.toml`

Drop a `.deadcode-ignore.toml` at the repo root. `deadcode` discovers it
by walking up from the scan root, like `.gitignore`.

```toml
# Most precise: ignore by exact stable Finding ID
[[ignore]]
id = "py:src/foo.py:42:unused_function:legacy_handler"
reason = "Called via reflection in worker dispatcher"

# By file glob + kind — kills the ORM/schema field bucket
[[ignore]]
file = "src/models/**.py"
kinds = ["unused_field", "unused_variable"]
reason = "Pydantic/SQLAlchemy field declarations"

# By symbol pattern across the repo
[[ignore]]
symbol = "*_at"
languages = ["python"]
kinds = ["unused_variable"]
reason = "ORM timestamp columns"
```

All matchers on a rule AND together; rules are evaluated in order and
the first match wins. `reason` is required. See
[`docs/SPEC.md`](docs/SPEC.md#unified-ignore-v02) for full semantics.

## Supported languages (v0.3)

| Language       | Tool      | Install                                              |
|----------------|-----------|------------------------------------------------------|
| Python         | `vulture` | `pip install vulture` (or `uv tool install vulture`) |
| TypeScript/JS  | `knip`    | `npm install -g knip` (or auto-fallback to `npx -y knip`) |
| Elixir         | `mix compile` + Elixir ≥ 1.18 type system | `brew install elixir` or [elixir-lang.org/install](https://elixir-lang.org/install.html) |
| Go             | `staticcheck -checks=U1000` | `go install honnef.co/go/tools/cmd/staticcheck@latest` |
| Go             | `golang.org/x/tools/cmd/deadcode` (second source) | `go install golang.org/x/tools/cmd/deadcode@latest` |

Run `deadcode doctor` to see which adapters are usable on your machine.
More adapters land in v0.3.x — see [`docs/SPEC.md`](docs/SPEC.md).

## Output schema

JSON output conforms to the schema documented in
[`docs/SPEC.md`](docs/SPEC.md#the-finding-schema). Schema version is exposed
as `schema_version` so consumers can branch on it.

## Tests

```bash
go test ./...
```
