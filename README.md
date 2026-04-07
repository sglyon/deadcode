# deadcode

Multi-language dead-code detection orchestrator. Detects languages, runs the
right per-language analyzer, and emits a unified, agent-friendly report.

**Status:** v0.2 — vertical slice (Python via `vulture`) plus the unified
`.deadcode-ignore.toml` filter that scales across every future adapter.
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
