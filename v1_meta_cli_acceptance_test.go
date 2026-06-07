package main

import "testing"

// TestV1MetaCLIOutsideInScenarios is an executable checklist for the first
// implementation pass. These are intentionally skipped until each slice is
// implemented; unskip one scenario at a time, watch it fail, then make it pass.
func TestV1MetaCLIOutsideInScenarios(t *testing.T) {
	scenarios := []struct {
		name string
		why  string
	}{
		{
			name: "version_json_contract",
			why:  "plane-cli version --format json emits valid schema-versioned JSON and exits 0",
		},
		{
			name: "config_get_redacts_secrets",
			why:  "config get shows effective non-secret config sources and never leaks PLANE_API_KEY",
		},
		{
			name: "config_set_rejects_secret_keys",
			why:  "config set api_key/pat/token fails with CONFIG_WRITE_REJECTED_SECRET",
		},
		{
			name: "auth_status_missing_config_is_typed_and_offline",
			why:  "auth status reports missing API key/workspace/base URL without making network calls",
		},
		{
			name: "auth_status_validates_pat_with_fake_plane",
			why:  "auth status calls /api/v1/users/me/ against a fake Plane server and reports identity/workspace status",
		},
		{
			name: "doctor_for_agent_is_actionable",
			why:  "doctor --for-agent --format json reports checks, config sources, redactions, and next actions",
		},
		{
			name: "resolve_readable_work_item_id",
			why:  "resolve ENG-42 parses the readable ID, queries a fake Plane server, and returns UUIDs",
		},
		{
			name: "resolve_invalid_reference_is_typed",
			why:  "resolve malformed references returns INVALID_REFERENCE with a fix and examples",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Skip("outside-in TDD scaffold: " + scenario.why)
		})
	}
}
