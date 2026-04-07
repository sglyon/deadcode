package elixir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectAppNameStandardForm(t *testing.T) {
	dir := t.TempDir()
	body := `defmodule MyApp.MixProject do
  use Mix.Project

  def project do
    [
      app: :my_app,
      version: "0.1.0"
    ]
  end
end
`
	if err := os.WriteFile(filepath.Join(dir, "mix.exs"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := projectAppName(dir); got != "my_app" {
		t.Errorf("projectAppName = %q, want my_app", got)
	}
}

func TestProjectAppNameUmbrella(t *testing.T) {
	// Umbrella root has apps_path, no top-level app:. We return ""
	// to keep all sections (noisier but we don't silently drop the
	// user's child-app warnings in v0.4).
	dir := t.TempDir()
	body := `defmodule Umbrella.MixProject do
  use Mix.Project

  def project do
    [
      apps_path: "apps",
      version: "0.0.1"
    ]
  end
end
`
	if err := os.WriteFile(filepath.Join(dir, "mix.exs"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := projectAppName(dir); got != "" {
		t.Errorf("umbrella projectAppName = %q, want empty", got)
	}
}

func TestProjectAppNameMissingFile(t *testing.T) {
	if got := projectAppName(t.TempDir()); got != "" {
		t.Errorf("missing mix.exs should return empty, got %q", got)
	}
}

func TestProjectAppNamePhoenixTemplate(t *testing.T) {
	// Mirrors the shape `mix phx.new` generates to ensure the regex
	// handles whitespace and comment lines correctly.
	dir := t.TempDir()
	body := `defmodule Arilearn.MixProject do
  use Mix.Project

  def project do
    [
      app: :arilearn,
      version: "0.1.0",
      elixir: "~> 1.14",
      elixirc_paths: elixirc_paths(Mix.env()),
      start_permanent: Mix.env() == :prod,
      aliases: aliases(),
      deps: deps()
    ]
  end
end
`
	if err := os.WriteFile(filepath.Join(dir, "mix.exs"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := projectAppName(dir); got != "arilearn" {
		t.Errorf("projectAppName = %q, want arilearn", got)
	}
}
