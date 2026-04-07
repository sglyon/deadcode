package python

import (
	"strings"
	"testing"
)

func TestBuildIgnoreDecoratorsFlagDefaults(t *testing.T) {
	flag := BuildIgnoreDecoratorsFlag(nil, true)
	if flag == "" {
		t.Fatal("expected default flag to be non-empty")
	}
	for _, want := range []string{"@app.*", "@router.*", "@property", "@pytest.fixture"} {
		if !strings.Contains(flag, want) {
			t.Errorf("default flag missing %q: %s", want, flag)
		}
	}
}

func TestBuildIgnoreDecoratorsFlagExtras(t *testing.T) {
	flag := BuildIgnoreDecoratorsFlag([]string{"@my.task", "@app.*"}, true)
	if !strings.Contains(flag, "@my.task") {
		t.Errorf("expected user pattern in flag: %s", flag)
	}
	// Dedupe: @app.* is in defaults too, must appear exactly once.
	if got := strings.Count(flag, "@app.*"); got != 1 {
		t.Errorf("expected @app.* exactly once, got %d", got)
	}
}

func TestBuildIgnoreDecoratorsFlagNoDefaults(t *testing.T) {
	flag := BuildIgnoreDecoratorsFlag([]string{"@only.this"}, false)
	if flag != "@only.this" {
		t.Errorf("expected only user pattern, got %q", flag)
	}
}

func TestBuildIgnoreDecoratorsFlagEmpty(t *testing.T) {
	if got := BuildIgnoreDecoratorsFlag(nil, false); got != "" {
		t.Errorf("expected empty flag, got %q", got)
	}
}
