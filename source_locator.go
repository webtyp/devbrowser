package devbrowser

// ElementSourceQuery contains element selectors and attributes for source resolution.
type ElementSourceQuery struct {
	Tag         string            `json:"tag"`
	ID          string            `json:"id"`
	Classes     []string          `json:"classes"`
	DataKey     string            `json:"data_key"`
	Attributes  map[string]string `json:"attributes"`
	Breadcrumbs []string          `json:"breadcrumbs"`
}

// SourceKind says what a SourceLocation points at.
type SourceKind string

const (
	SourceRender      SourceKind = "render"      // the line that puts the class into the DOM
	SourceStyle       SourceKind = "style"       // the stylesheet line for the class (a css.go file)
	SourceDeclaration SourceKind = "declaration" // the widget identity: const NameX = widget.Name("x")
	SourceText        SourceKind = "text"        // a literal match of a class/id/key outside widget identity
)

// SourceLocation is one place in local source code related to a DOM element.
type SourceLocation struct {
	File   string     `json:"file"` // as shown to a person: "~/..." (wpath.Tilde from webtyp.com/filepath)
	Line   int        `json:"line"`
	Kind   SourceKind `json:"kind"`
	Token  string     `json:"token"`  // the DOM token that led here, e.g. "composebar__row" or "ancestor composebar"
	Match  string     `json:"match"`  // the trimmed source line
	Origin string     `json:"origin"` // "project", "webtyp.com/components (local checkout; project pins v0.8.7)", ...
}

// SourceLocator resolves a DOM element's identifiers to local source code files.
type SourceLocator interface {
	LocateSource(query ElementSourceQuery) []SourceLocation
}
