package python

// DefaultIgnoreDecorators is the built-in list of decorator glob patterns
// that mark a Python function/class as "called by the framework" rather
// than dead code. Vulture cannot reason about decorator-driven dispatch,
// so without this list nearly every FastAPI/Flask/Django/Click/Celery
// project produces hundreds of false positives.
//
// Curated for breadth without bloat — covers the patterns that account
// for the vast majority of real-world false positives. Users can extend
// this list with --ignore-decorators on the CLI.
//
// Pattern syntax is vulture's: glob wildcards (*, ?, [abc]) on the
// `@dotted.name` form.
var DefaultIgnoreDecorators = []string{
	// Web frameworks: app/router method decorators (FastAPI, Flask, Sanic, Bottle, ...)
	"@app.*",
	"@router.*",
	"@blueprint.*",
	"@bp.*",
	"@*.route",
	"@*.get",
	"@*.post",
	"@*.put",
	"@*.delete",
	"@*.patch",
	"@*.head",
	"@*.options",
	"@*.websocket",
	"@*.middleware",
	"@*.exception_handler",
	"@*.errorhandler",
	"@*.before_request",
	"@*.after_request",
	"@*.on_event",

	// pytest
	"@pytest.fixture",
	"@pytest.mark.*",
	"@fixture",

	// Click / Typer CLIs
	"@click.command",
	"@click.group",
	"@*.command",
	"@*.group",

	// Celery / RQ / Huey / Dramatiq task queues
	"@*.task",
	"@*.shared_task",
	"@*.periodic_task",
	"@*.actor",

	// Django signals and template tags
	"@receiver",
	"@*.connect",
	"@register.*",

	// Python descriptors — cached_property, property setters, etc.
	"@property",
	"@*.setter",
	"@*.deleter",
	"@*.getter",
	"@cached_property",
	"@functools.cached_property",

	// dataclass-style decorators that synthesize methods
	"@dataclass",
	"@dataclasses.dataclass",
	"@attr.s",
	"@attrs.define",
	"@pydantic.validator",
	"@pydantic.field_validator",
	"@validator",
	"@field_validator",
	"@model_validator",
	"@root_validator",
}

// BuildIgnoreDecoratorsFlag returns the comma-separated value to pass
// to `vulture --ignore-decorators`. Returns an empty string if no
// patterns apply (caller should then omit the flag entirely).
func BuildIgnoreDecoratorsFlag(extra []string, useDefaults bool) string {
	var patterns []string
	if useDefaults {
		patterns = append(patterns, DefaultIgnoreDecorators...)
	}
	patterns = append(patterns, extra...)
	if len(patterns) == 0 {
		return ""
	}
	// Dedupe while preserving order so the resulting flag is stable.
	seen := make(map[string]bool, len(patterns))
	out := patterns[:0]
	for _, p := range patterns {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return joinComma(out)
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	n := len(parts) - 1
	for _, p := range parts {
		n += len(p)
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, p...)
	}
	return string(b)
}
