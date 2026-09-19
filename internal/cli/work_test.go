package cli

import (
	"reflect"
	"testing"
)

func TestStripHTMLNormalizesText(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{name: "nested tags and whitespace", html: "<p>Hello <strong>world</strong></p>\n<p>Next</p>", want: "Hello world Next"},
		{name: "escaped angle brackets", html: "<p>Use &lt;tag&gt; &amp; continue</p>", want: "Use <tag> & continue"},
		{name: "named and numeric entities", html: "<p>&copy; &#9731; &quot;ok&quot;</p>", want: `© ☃ "ok"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripHTML(tc.html); got != tc.want {
				t.Fatalf("stripHTML() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreserveWorkItemIdentity(t *testing.T) {
	original := workItemSummary{
		ProjectIdentifier: "BACKEND",
		ReadableID:        "BACKEND-42",
		ProjectID:         "project-backend",
		WorkItemID:        "work-42",
		SequenceID:        "42",
		Name:              "Fix OAuth",
		StateID:           "state-started",
		StateGroup:        "started",
	}

	cases := []struct {
		name    string
		updated workItemSummary
		want    workItemSummary
	}{
		{
			name: "fills missing identity from original",
			updated: workItemSummary{
				Name:       "Fix OAuth",
				StateID:    "state-cancelled",
				StateGroup: "cancelled",
			},
			want: workItemSummary{
				ProjectIdentifier: "BACKEND",
				ReadableID:        "BACKEND-42",
				ProjectID:         "project-backend",
				WorkItemID:        "work-42",
				SequenceID:        "42",
				Name:              "Fix OAuth",
				StateID:           "state-cancelled",
				StateGroup:        "cancelled",
			},
		},
		{
			name: "keeps identity returned by update",
			updated: workItemSummary{
				ProjectIdentifier: "OPS",
				ReadableID:        "OPS-7",
				ProjectID:         "project-ops",
				WorkItemID:        "work-7",
				SequenceID:        "7",
				Name:              "Fix OAuth",
				StateID:           "state-done",
				StateGroup:        "completed",
			},
			want: workItemSummary{
				ProjectIdentifier: "OPS",
				ReadableID:        "OPS-7",
				ProjectID:         "project-ops",
				WorkItemID:        "work-7",
				SequenceID:        "7",
				Name:              "Fix OAuth",
				StateID:           "state-done",
				StateGroup:        "completed",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := preserveWorkItemIdentity(tc.updated, original)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("preserveWorkItemIdentity() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
