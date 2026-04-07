# deadcode — Architecture Specification

**Status:** Draft v0.1
**Last updated:** 2026-04-07

## Thesis

Cross-language dead code detection is a real, under-served gap. Existing tools are excellent **per language** (vulture, knip, staticcheck, cargo-udeps, ...) but each has its own CLI, output format, and idiosyncrasies. There is no unified, agent-friendly multi-language entry point.

`deadcode` does not reinvent semantic analysis. It is an **orchestrator**:

> Detect languages → run the right per-language analyzer → normalize output → present unified, actionable findings.

The novel value is integration, normalization, and agent-friendliness — not parsing.

## Non-Goals

- We do **not** write language parsers or call-graph analyzers ourselves.
- We do **not** try to beat best-in-class per-language tools at their own game.
- We do **not** auto-fix code in v1. (Maybe later. Schema reserves a `fix_hint` field.)
- We do **not** ship duplication detection. That's `jscpd`'s job.

## Design Constraints

1. **Multi-language out of the box** — useful on the first run in a polyglot repo.
2. **Agent-friendly CLI** — structured JSON, stable IDs, `path:line` everywhere, deterministic ordering, short error messages.
3. **Single static binary** — `brew install deadcode` or `curl | sh`. No runtime, no venv.
4. **Honest about confidence** — different tools have different precision; we surface that, not paper over it.
5. **Sane defaults** — first run requires zero config and produces useful output.
6. **Self-healing** — `deadcode doctor` tells the user exactly what's missing and how to install it.

## Architecture

```
            ┌─────────────────────────────────────┐
            │            deadcode CLI             │
            └──────────────┬──────────────────────┘
                           │
               ┌───────────┴────────────┐
               │  1. Language Detector  │  ← walks repo, classifies files
               └───────────┬────────────┘
                           │
               ┌───────────┴────────────┐
               │  2. Tool Resolver      │  ← which adapter for which language
               │     + availability     │     + check if tool installed
               └───────────┬────────────┘
                           │
               ┌───────────┴────────────┐
               │  3. Runner (parallel)  │  ← spawns analyzers, captures output
               └───────────┬────────────┘
                           │
               ┌───────────┴────────────┐
               │  4. Normalizer         │  ← per-tool adapter → Finding schema
               └───────────┬────────────┘
                           │
               ┌───────────┴────────────┐
               │  5. Filter / Rank      │  ← confidence, kind, public API
               └───────────┬────────────┘
                           │
               ┌───────────┴────────────┐
               │  6. Reporters          │  ← json, console, sarif, markdown
               └────────────────────────┘
```

The core seam is the **Adapter interface** (step 2-4). Adding a language is implementing one interface.

## The Finding Schema

The load-bearing artifact. All adapters normalize to this; all reporters consume it.

```json
{
  "schema_version": "1",
  "summary": {
    "languages": ["python", "typescript", "go"],
    "files_scanned": 1247,
    "findings_total": 38,
    "tools_run": ["vulture", "knip", "staticcheck"],
    "tools_unavailable": ["cargo-udeps"],
    "duration_ms": 4210
  },
  "findings": [
    {
      "id": "py:src/foo.py:42:unused_function:compute_widget_total",
      "file": "/abs/path/to/src/foo.py",
      "line": 42,
      "end_line": 58,
      "symbol": "compute_widget_total",
      "kind": "unused_function",
      "language": "python",
      "tool": "vulture",
      "confidence": 0.85,
      "message": "Function 'compute_widget_total' appears unused",
      "evidence": {
        "tool_raw": "src/foo.py:42: unused function 'compute_widget_total' (85% confidence)",
        "tool_confidence": "85%"
      },
      "fix_hint": "delete"
    }
  ]
}
```

### Field semantics

| Field | Notes |
|---|---|
| `schema_version` | Bumped on breaking changes. Consumers should branch on it. |
| `id` | Stable across runs. Format: `<lang-prefix>:<rel-path>:<line>:<kind>:<symbol>`. Used for diffing and `explain`. |
| `file` | Always absolute. Agents need this for direct file access. |
| `line` / `end_line` | 1-indexed. `end_line` optional. |
| `kind` | Closed enum (see below). Agents branch on this. |
| `confidence` | Normalized 0–1. Per-tool mapping in adapter. |
| `evidence.tool_raw` | Original tool output line. Audit trail and provenance. |
| `fix_hint` | Closed enum: `delete`, `make-private`, `inline`, `manual`. v1 informational only. |

### Kind enum

```
unused_function
unused_method
unused_class
unused_variable
unused_constant
unused_import
unused_export
unused_param
unused_field
unreachable
unused_dependency
```

Adapters MUST map to one of these. Unknown kinds → adapter bug, fail loudly.

### Confidence normalization

| Tool | Native signal | Mapped confidence |
|---|---|---|
| vulture | `60%`–`100%` numeric | divide by 100 |
| knip | binary present | `0.95` |
| staticcheck | binary present (`U1000`) | `0.95` |
| cargo-udeps | binary present | `0.90` |
| ruff (F401, F841) | binary present | `0.95` |
| cppcheck | binary present | `0.85` |
| Heuristic fallback | grep-based | `0.40` |

Honest, per-tool, documented in code.

## Adapter Interface

```go
package adapter

type Adapter interface {
    // Name of the analyzer (e.g., "vulture", "knip").
    Name() string

    // Languages this adapter handles (e.g., []string{"python"}).
    Languages() []string

    // Tool availability check. Returns nil if usable.
    // On failure, returns an error with a human-readable install hint.
    Check(ctx context.Context) error

    // Run the analyzer over the given paths and parse the output
    // into normalized Findings.
    Run(ctx context.Context, paths []string, opts RunOptions) ([]Finding, error)
}

type RunOptions struct {
    ExcludeTests bool
    Verbose      bool
}
```

This is the only extension point. Adding a language is ~50 lines plus a parser.

## Per-language adapter plan

| Language     | Tool                              | Why |
|--------------|-----------------------------------|---|
| Python       | `vulture` (+ `ruff F401/F841`)    | Native % confidence; widely installed |
| TypeScript   | `knip`                            | Modern, project-aware, JSON output |
| JavaScript   | `knip`                            | Same |
| Go           | `staticcheck -checks=U1000`       | Standard, accurate, JSON output |
| Rust         | `cargo +nightly udeps` (deps) + `cargo check` warnings | Best available |
| Java         | `pmd` UnusedPrivateMethod ruleset | No IDE dependency |
| Ruby         | `debride`                         | Only real option |
| C/C++        | `cppcheck --enable=unusedFunction`| Standard; SARIF output |
| C#           | `dotnet format analyzers` (IDE0051) | Roslyn, ships with SDK |
| Elixir       | `mix xref unreachable`            | Built into Elixir |
| **Fallback** | Heuristic symbol grep             | For anything else; low confidence |

## Public API awareness

The biggest source of false positives is library code where symbols are consumed externally. Handled in three layers:

1. **Auto-detection** of public surfaces:
   - `package.json` `"exports"` field → exports are "used"
   - `pyproject.toml` `[project.entry-points]` → entry points are "used"
   - `Cargo.toml` `[lib]` + `pub` items → "used"
   - Go: capitalized symbols in non-`internal/` packages → "used" (with a flag to override)

2. **`.deadcoderc` config** for explicit declarations:
   ```yaml
   entry_points:
     - src/main.py
     - bin/cli
   public_api:
     - "src/lib/**/__init__.py"
     - pattern: "^[A-Z]"
   exclude:
     - "tests/**"
     - "**/migrations/**"
   ```

3. **Test-aware filtering** — by default, code only used by tests gets a `test_only: true` marker but is still emitted. User chooses whether that counts as "used."

This layer is what turns "noisy raw tool output" into "actionable findings." Most of the engineering value lives here.

## CLI surface

```
deadcode [path]                        # Default: scan, console output
deadcode scan [path]                   # Explicit scan
deadcode doctor                        # Tool availability check
deadcode adapters                      # List supported languages and tools
deadcode ignore list                   # Print rules in the discovered ignore file
deadcode ignore validate               # Parse + validate the ignore file
deadcode explain <id>                  # (v0.4) Full evidence dump for one finding
```

### Scan flags

```
--json                  Emit JSON instead of console output
-o, --output <path>     Write report to file (default stdout)
--lang <list>           Restrict to languages: python,typescript,go
--kind <list>           Restrict to kinds: unused_function,unused_import
--min-confidence <f>    Filter findings below this confidence (0.0–1.0)
--exclude-tests         Drop findings whose only callers are tests
--ignore-decorators <l> Add Python decorator glob patterns to the ignore list
--no-default-decorators Disable the built-in framework decorator list
--ignore-file <path>    Use a specific .deadcode-ignore.toml
--no-ignore-file        Disable ignore-file loading entirely
--show-ignored          Print suppressed findings (with reasons)
--since <ref>           (v0.4) Diff mode: only findings introduced since git ref
--threshold <n>         Fail if findings count exceeds this
--exit-code <n>         Exit code when threshold exceeded (default 0)
-v, --verbose           Verbose output (preserves console reporter chatter)
```

### Agent-default invocation

```bash
deadcode scan --json --output /tmp/deadcode.json .
```

Then `Read /tmp/deadcode.json`, group by file, present top findings by confidence with `path:line` citations.

## Implementation choices

- **Language: Go.** Single static binary. Excellent subprocess + JSON handling. Fast startup (matters for skill-driven invocation). Easy cross-compilation.
- **CLI framework:** stdlib `flag` for v0.1–v0.2 (still hermetic enough). Migrate to Cobra in v0.3 if subcommand sprawl warrants it.
- **TOML parser:** `github.com/BurntSushi/toml` (the standard Go TOML library).
- **Glob matching:** `github.com/bmatcuk/doublestar/v4` for `**` support.
- **Adapter delivery:** built-in, compiled into the binary. No runtime plugin loading in v1. External adapters via shell-script contract reserved for v0.4+.
- **Concurrency:** one goroutine per adapter run; `errgroup` for coordination. Adapters are independent.
- **Output ordering:** sort findings by `(file, line)` for deterministic diffs.

## Versioning roadmap

### v0.1 — Vertical slice (this commit)
- Go binary
- 1 adapter: Python (`vulture`)
- Finding schema + JSON reporter + console reporter
- `scan` and `doctor` subcommands
- `--exclude-tests` flag
- No public-API config yet

**Goal:** prove the architecture end-to-end with one real language. Usable from a Claude skill on day one.

### v0.2 — Unified ignore mechanism (current)
- `.deadcode-ignore.toml` filter (see "Unified ignore" below)
- `deadcode ignore list / validate` subcommand
- `--ignore-file`, `--no-ignore-file`, `--show-ignored` scan flags
- Auto-discovery of ignore file by walking up from scan root
- Project-root detection (`.git`, `pyproject.toml`, `package.json`,
  `Cargo.toml`, `go.mod`) for portable file globs
- Validated against bd-tracker: 137 → 16 findings (88% reduction)

### v0.3 — Breadth
- Add TypeScript (`knip`) and Go (`staticcheck`) adapters
- Confidence normalization documented per-adapter
- Markdown reporter
- Migrate to Cobra if subcommand sprawl warrants it

### v0.4 — Agent integration
- `--since <ref>` diff mode
- `explain <id>` subcommand
- SARIF reporter (for GitHub code scanning)
- Heuristic fallback adapter for unsupported languages
- `deadcode ignore add <id>` writer
- `deadcode ignore stats` (which rules are doing work)

### v0.5 — Polish
- Auto-detect entry points from `package.json`, `pyproject.toml`, `Cargo.toml`
- Result caching between runs
- Brew formula and install script
- Companion `deadcode` Claude skill (mirrors the `jscpd` skill)

## Unified ignore (v0.2)

The `.deadcode-ignore.toml` file is the canonical mechanism for
suppressing findings across every language and tool. It lives at the
repo root and is loaded automatically by walking upward from the scan
root, like `.gitignore`.

```toml
# Ignore by exact stable Finding ID — most precise, survives refactors
[[ignore]]
id = "py:src/foo.py:42:unused_function:legacy_handler"
reason = "Called via reflection in worker dispatcher"

# Ignore by file glob + kind — kills the ORM/schema bucket
[[ignore]]
file = "src/models/**.py"
kinds = ["unused_field", "unused_variable"]
reason = "Pydantic/SQLAlchemy field declarations"

# Ignore by symbol pattern across the repo
[[ignore]]
symbol = "*_at"
languages = ["python"]
kinds = ["unused_variable"]
reason = "ORM timestamp columns"
```

### Matcher semantics

A rule matches a finding when **every** specified field matches:

| Field       | Type    | Match against           | Notes |
|-------------|---------|-------------------------|-------|
| `id`        | string  | exact `Finding.ID`      | Most precise |
| `file`      | glob    | path or basename        | Doublestar globs (`**`) |
| `symbol`    | glob    | `Finding.Symbol`        | Doublestar globs |
| `kinds`     | list    | `Finding.Kind`          | Any-of |
| `languages` | list    | `Finding.Language`      | Any-of |
| `tools`     | list    | `Finding.Tool`          | Any-of |
| `reason`    | string  | (not matched)           | **Required** for documentation |

A rule with no matchers matches nothing — `deadcode ignore validate`
catches this.

### Resolution order

1. `--ignore-file <path>` — explicit, must exist
2. Discovered file (walk upward from scan root for `.deadcode-ignore.toml`)
3. None (no filter)

`--no-ignore-file` disables loading entirely.

### File-glob roots

File globs are resolved against multiple candidate roots so the same
ignore file works whether you scan the whole repo or a subdirectory:

1. The absolute file path itself
2. The directory containing the ignore file (the repo root when the
   file is committed at repo root)
3. Each scan path passed via CLI args
4. The detected project root for each scan path (nearest ancestor
   containing `.git`, `pyproject.toml`, `package.json`, `Cargo.toml`,
   or `go.mod`)
5. The basename (so `**.py` works anywhere)

A glob matches if **any** of these resolutions matches. False positives
are extremely unlikely because all matchers on a rule must AND together.

### First-match-wins

Rules are evaluated in order and the first match wins. Order specific
rules above general ones if you want them to attribute correctly. The
`--show-ignored` flag prints which rule (by index) suppressed each
finding so you can audit the ruleset.

## Open questions

1. **`--since` semantics** — should it diff finding IDs, or re-run only changed files? The former is more informative; the latter is faster. Likely both, with `--since-fast` as a separate flag.
2. **Public-API auto-detection** — how aggressive by default? Lean conservative (treat more things as "used") to keep false-positive rate low.
3. **Exit codes for CI** — match `jscpd`'s `--threshold` + `--exit-code` pattern? Almost certainly yes.
4. **Schema stability** — when do we lock `schema_version` to "1"? After v0.3 once SARIF and heuristic adapters are in.
