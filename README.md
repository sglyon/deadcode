# deadcode

Multi-language dead-code detection orchestrator. Detects languages, runs the
right per-language analyzer, and emits a unified, agent-friendly report.

**Status:** v0.1 — vertical slice. One adapter (Python via `vulture`).
See [`docs/SPEC.md`](docs/SPEC.md) for the full architecture and roadmap.

## Why

Per-language dead-code tools are excellent (vulture, knip, staticcheck,
cargo-udeps, ...) but each has its own CLI and output format. There is no
unified, agent-friendly multi-language entry point. `deadcode` doesn't
reinvent semantic analysis — it orchestrates the best-in-class tools and
normalizes their output.

For code duplication, use [`jscpd`](https://github.com/kucherenko/jscpd)
instead. `deadcode` is dead-code only.

## Build

```bash
go build -o deadcode .
```

## Usage

```bash
# Default: scan current directory, console output
deadcode

# JSON to a file (agent-friendly)
deadcode scan --json --output /tmp/deadcode.json .

# Restrict by language and confidence
deadcode scan --lang python --min-confidence 0.8 src/

# CI gate
deadcode scan --threshold 0 --exit-code 1 --json .

# Check which underlying tools are installed
deadcode doctor

# List built-in adapters
deadcode adapters
```

## Supported languages (v0.1)

| Language | Tool      | Install                |
|----------|-----------|------------------------|
| Python   | `vulture` | `pip install vulture`  |

More adapters land in v0.2 — see [`docs/SPEC.md`](docs/SPEC.md).

## Output schema

JSON output conforms to the schema documented in
[`docs/SPEC.md`](docs/SPEC.md#the-finding-schema). Schema version is exposed
as `schema_version` so consumers can branch on it.

## Tests

```bash
go test ./...
```
