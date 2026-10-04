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

// SourceLocation represents the local source file and line where an element is defined.
type SourceLocation struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet,omitempty"`
	Match   string `json:"match,omitempty"`
}

// SourceLocator resolves a DOM element's identifiers to local source code files.
type SourceLocator interface {
	LocateSource(query ElementSourceQuery) []SourceLocation
}
