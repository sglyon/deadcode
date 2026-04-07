// Package finding defines the unified finding schema that all adapters
// normalize to and all reporters consume.
//
// The schema is documented in docs/SPEC.md. Bump SchemaVersion on any
// breaking change so consumers can branch on it.
package finding

const SchemaVersion = "1"

// Kind is a closed enum. Adapters MUST map their native concepts to one
// of these. Unknown kinds are an adapter bug — fail loudly at the seam.
type Kind string

const (
	KindUnusedFunction   Kind = "unused_function"
	KindUnusedMethod     Kind = "unused_method"
	KindUnusedClass      Kind = "unused_class"
	KindUnusedType       Kind = "unused_type"
	KindUnusedVariable   Kind = "unused_variable"
	KindUnusedConstant   Kind = "unused_constant"
	KindUnusedImport     Kind = "unused_import"
	KindUnusedExport     Kind = "unused_export"
	KindUnusedParam      Kind = "unused_param"
	KindUnusedField      Kind = "unused_field"
	KindUnusedFile       Kind = "unused_file"
	KindUnreachable      Kind = "unreachable"
	KindUnusedDependency Kind = "unused_dependency"
)

// FixHint is informational in v1 — reserved for a future autofix mode.
type FixHint string

const (
	FixDelete      FixHint = "delete"
	FixMakePrivate FixHint = "make-private"
	FixInline      FixHint = "inline"
	FixManual      FixHint = "manual"
)

// Finding is the normalized representation of a single dead-code report
// from any adapter. See docs/SPEC.md for field semantics.
type Finding struct {
	ID         string            `json:"id"`
	File       string            `json:"file"`
	Line       int               `json:"line"`
	EndLine    int               `json:"end_line,omitempty"`
	Symbol     string            `json:"symbol,omitempty"`
	Kind       Kind              `json:"kind"`
	Language   string            `json:"language"`
	Tool       string            `json:"tool"`
	Confidence float64           `json:"confidence"`
	Message    string            `json:"message"`
	Evidence   map[string]string `json:"evidence,omitempty"`
	FixHint    FixHint           `json:"fix_hint,omitempty"`
}

// Summary is the top-level metadata block in a Report.
type Summary struct {
	Languages        []string `json:"languages"`
	FilesScanned     int      `json:"files_scanned"`
	FindingsTotal    int      `json:"findings_total"`
	FindingsIgnored  int      `json:"findings_ignored,omitempty"`
	ToolsRun         []string `json:"tools_run"`
	ToolsUnavailable []string `json:"tools_unavailable,omitempty"`
	IgnoreFile       string   `json:"ignore_file,omitempty"`
	DurationMs       int64    `json:"duration_ms"`
}

// IgnoredFinding wraps a Finding with the reason it was suppressed.
// Only present in the report when --show-ignored is set.
type IgnoredFinding struct {
	Finding
	IgnoreReason string `json:"ignore_reason"`
	MatchedRule  int    `json:"matched_rule"`
}

// Report is the full output document.
type Report struct {
	SchemaVersion string           `json:"schema_version"`
	Summary       Summary          `json:"summary"`
	Findings      []Finding        `json:"findings"`
	Ignored       []IgnoredFinding `json:"ignored,omitempty"`
}
