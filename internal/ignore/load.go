package ignore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// FileName is the canonical name of the unified ignore file. Searched
// upward from the scan root, like .gitignore.
const FileName = ".deadcode-ignore.toml"

// Load reads a Ruleset from an explicit path. Returns an empty Ruleset
// (not nil) if the file does not exist and required is false.
func Load(path string, required bool) (*Ruleset, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return &Ruleset{}, nil
		}
		return nil, fmt.Errorf("reading ignore file %s: %w", abs, err)
	}
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", abs, err)
	}
	rs := &Ruleset{
		Path:  abs,
		Dir:   filepath.Dir(abs),
		Rules: f.Ignore,
	}
	return rs, nil
}

// Discover walks upward from start looking for FileName. Returns an
// empty Ruleset if none found. Stops at the filesystem root.
func Discover(start string) (*Ruleset, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return Load(candidate, true)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return &Ruleset{}, nil // hit filesystem root, no file
		}
		dir = parent
	}
}
