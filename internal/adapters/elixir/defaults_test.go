package elixir

import (
	"testing"

	"github.com/sglyon/deadcode/internal/finding"
)

func TestMatchesAnyDefault(t *testing.T) {
	cases := map[string]bool{
		// Phoenix controller actions — should match
		"MyApp.UserController.index/2":     true,
		"MyApp.UserController.show/2":      true,
		"MyApp.UserController.create/2":    true,
		"DeepNs.Web.PageController.show/2": true,

		// GenServer callbacks
		"MyApp.Worker.handle_call/3": true,
		"MyApp.Worker.init/1":        true,
		"MyApp.Worker.terminate/2":   true,

		// LiveView
		"MyApp.UserLive.mount/3":         true,
		"MyApp.UserLive.handle_event/3":  true,
		"MyApp.SomeLiveView.render/1":    true,
		"MyApp.RowComponent.render/1":    true,
		"MyApp.RowComponent.update/2":    true,

		// Channel
		"MyApp.RoomChannel.join/3":      true,
		"MyApp.RoomChannel.handle_in/3": true,

		// Plug
		"MyApp.AuthPlug.call/2": true,

		// Should NOT match — real dead code that happens to look phoenixish
		"MyApp.Helpers.format_date/1":        false,
		"MyApp.Math.add/2":                   false,
		"MyApp.Orphan.private_dead/0":        false,
		"MyApp.Controller.helper_function/0": false, // not an action name
	}
	for sym, want := range cases {
		if got := matchesAnyDefault(sym); got != want {
			t.Errorf("matchesAnyDefault(%q) = %v, want %v", sym, got, want)
		}
	}
}

func TestApplyDefaultIgnoresKeepsRealFindings(t *testing.T) {
	in := []finding.Finding{
		{Symbol: "MyApp.UserController.index/2", Kind: finding.KindUnusedFunction},
		{Symbol: "MyApp.Helpers.format_date/1", Kind: finding.KindUnusedFunction},
		{Symbol: "MyApp.Worker.handle_call/3", Kind: finding.KindUnusedFunction},
		{Symbol: "MyApp.Orphan.private_dead/0", Kind: finding.KindUnusedFunction},
	}
	out := applyDefaultIgnores(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 findings to survive defaults, got %d: %+v", len(out), out)
	}
	survivedSymbols := map[string]bool{}
	for _, f := range out {
		survivedSymbols[f.Symbol] = true
	}
	if !survivedSymbols["MyApp.Helpers.format_date/1"] {
		t.Error("real helper finding was incorrectly suppressed")
	}
	if !survivedSymbols["MyApp.Orphan.private_dead/0"] {
		t.Error("real orphan finding was incorrectly suppressed")
	}
}
