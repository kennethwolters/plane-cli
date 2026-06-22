package main

import "testing"

func TestWorkItemWithResolvedStateGroup(t *testing.T) {
	stateGroupsByID := map[string]string{
		"state-backlog": "backlog",
		"state-started": "started",
	}
	cases := []struct {
		name string
		item workItemSummary
		want string
	}{
		{
			name: "fills missing group from state id",
			item: workItemSummary{StateID: "state-backlog"},
			want: "backlog",
		},
		{
			name: "preserves existing group",
			item: workItemSummary{StateID: "state-backlog", StateGroup: "completed"},
			want: "completed",
		},
		{
			name: "leaves unknown state id empty",
			item: workItemSummary{StateID: "state-unknown"},
			want: "",
		},
		{
			name: "leaves empty state id empty",
			item: workItemSummary{},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := workItemWithResolvedStateGroup(tc.item, stateGroupsByID)
			if got.StateGroup != tc.want {
				t.Fatalf("StateGroup = %q, want %q", got.StateGroup, tc.want)
			}
		})
	}
}
