// Root-level test (justified): issueMessage formats a CDP event that only arrives from a running Chrome; no public API returns it.
package devbrowser

import (
	"testing"

	"webtyp.com/devbrowser/cdproto/audits"
)

func TestIssueMessage(t *testing.T) {
	cases := []struct {
		name  string
		issue *audits.InspectorIssue
		want  string
	}{
		{"code only", &audits.InspectorIssue{Code: audits.InspectorIssueCodeMixedContentIssue}, "[Issue] MixedContentIssue"},
		{"generic with detail", &audits.InspectorIssue{
			Code: audits.InspectorIssueCodeGenericIssue,
			Details: &audits.InspectorIssueDetails{GenericIssueDetails: &audits.GenericIssueDetails{
				ErrorType: audits.GenericIssueErrorTypeFormLabelForNameError, ViolatingNodeAttribute: "for",
			}},
		}, "[Issue] GenericIssue FormLabelForNameError (for)"},
	}
	for _, c := range cases {
		if got := issueMessage(c.issue); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
