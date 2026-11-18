// Package report holds the two value types every use case returns beside
// its data: a Decision (a resolved value and its source, spec 6.6) and a
// Warning. They live apart from internal/render so the app layer can build
// them without importing the renderer (internal/archtest).
package report

// Decision is one resolved value and where it came from.
type Decision struct {
	// Name is the concern ("agent", "profile", "login", "env", …).
	Name string `json:"name"`
	// Value is the decided value, redacted when secret.
	Value string `json:"value"`
	// Source says which input decided it ("--profile", "folder binding
	// ~/p", "default_agent").
	Source string `json:"source"`
}

// Warning is one non-fatal note.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
