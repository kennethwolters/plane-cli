package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue10WorkEditAppendDescriptionDryRunShowsBeforeAfter(t *testing.T) {
	const apiKey = "issue10-append-secret"
	server := fakeIssue10RemainingPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "edit", "BACKEND-42", "--append-description-html", "<hr><p>Follow-up</p>", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.edit.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["applied"] != false {
		t.Fatalf("dry-run should not apply: %#v", data)
	}
	operation := data["operation"].(map[string]any)
	before := operation["before"].(map[string]any)
	after := operation["after"].(map[string]any)
	if before["description_html"] != "<p>OAuth bug</p>" || after["description_html"] != "<p>OAuth bug</p><hr><p>Follow-up</p>" {
		t.Fatalf("unexpected before/after: %#v / %#v", before, after)
	}
}

func TestIssue10WorkListCompactFieldsAndLatestComments(t *testing.T) {
	const apiKey = "issue10-board-secret"
	server := fakeIssue10RemainingPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "list", "--project", "BACKEND", "--fields", "readable_id,name,state_group,priority,description_excerpt,latest_comments", "--description-excerpt", "12", "--include-comments-latest", "1", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	items := env["data"].(map[string]any)["work_items"].([]any)
	if len(items) != 2 {
		t.Fatalf("unexpected items: %#v", items)
	}
	first := items[0].(map[string]any)
	if first["description_excerpt"] != "OAuth bug wi" {
		t.Fatalf("unexpected excerpt: %#v", first)
	}
	comments := first["latest_comments"].([]any)
	if len(comments) != 1 || comments[0].(map[string]any)["excerpt"] != "Investigate callback" {
		t.Fatalf("unexpected latest comments: %#v", comments)
	}
	if _, ok := first["work_item_id"]; ok {
		t.Fatalf("field projection included unrequested work_item_id: %#v", first)
	}
}

func TestIssue10WorkEditMetadataAndMoveState(t *testing.T) {
	const apiKey = "issue10-metadata-secret"
	state := newIssue10RemainingState()
	server := fakeIssue10RemainingPlaneWithState(t, apiKey, state)
	defer server.Close()

	edit := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "edit", "BACKEND-42", "--labels-add", "tracking", "--labels-remove", "needs-triage", "--assignees-add", "alice@example.test", "--assignees-remove", "bob@example.test", "--apply", "--verify", "--format", "json")
	edit.assertExit(t, 0)
	editEnv := parseEnvelope(t, edit.stdout)
	editData := editEnv["data"].(map[string]any)
	if editData["applied"] != true || editData["verified"] != true {
		t.Fatalf("metadata edit did not apply and verify: %#v", editData)
	}
	if state.lastPatch["labels"] == nil || state.lastPatch["assignees"] == nil {
		t.Fatalf("metadata patch did not include labels and assignees: %#v", state.lastPatch)
	}

	move := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "move", "BACKEND-42", "--state", "Ready", "--apply", "--verify", "--format", "json")
	move.assertExit(t, 0)
	moveEnv := parseEnvelope(t, move.stdout)
	moveData := moveEnv["data"].(map[string]any)
	if moveEnv["schema"] != "plane.work.move.v1" || moveData["applied"] != true || moveData["verified"] != true {
		t.Fatalf("move did not apply and verify: %#v", moveEnv)
	}
	if state.lastPatch["state"] != "state-ready" {
		t.Fatalf("move did not patch ready state: %#v", state.lastPatch)
	}
}

func TestIssue10WorkBatchEditAndComment(t *testing.T) {
	const apiKey = "issue10-batch-secret"
	state := newIssue10RemainingState()
	server := fakeIssue10RemainingPlaneWithState(t, apiKey, state)
	defer server.Close()

	dir := t.TempDir()
	updatesPath := filepath.Join(dir, "updates.json")
	if err := os.WriteFile(updatesPath, []byte(`[{"key":"BACKEND-42","title":"Retitled OAuth","priority":"high"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	dryRun := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "batch", "edit", "--file", updatesPath, "--dry-run", "--format", "json")
	dryRun.assertExit(t, 0)
	dryEnv := parseEnvelope(t, dryRun.stdout)
	dryData := dryEnv["data"].(map[string]any)
	if dryEnv["schema"] != "plane.work.batch.edit.v1" || dryData["planned"] != float64(1) || dryData["applied"] != float64(0) {
		t.Fatalf("unexpected batch dry-run: %#v", dryEnv)
	}
	if state.editPatches != 0 {
		t.Fatalf("dry-run patched server: %#v", state)
	}

	commentsPath := filepath.Join(dir, "comments.json")
	if err := os.WriteFile(commentsPath, []byte(`[{"key":"BACKEND-42","html":"<p>Status: classified.</p>"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	applied := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "batch", "comment", "--file", commentsPath, "--apply", "--verify", "--format", "json")
	applied.assertExit(t, 0)
	appliedEnv := parseEnvelope(t, applied.stdout)
	appliedData := appliedEnv["data"].(map[string]any)
	if appliedEnv["schema"] != "plane.work.batch.comment.v1" || appliedData["applied"] != float64(1) || appliedData["failed"] != float64(0) {
		t.Fatalf("unexpected batch comment apply: %#v", appliedEnv)
	}
}

func TestIssue10WorkRelations(t *testing.T) {
	const apiKey = "issue10-relations-secret"
	state := newIssue10RemainingState()
	server := fakeIssue10RemainingPlaneWithState(t, apiKey, state)
	defer server.Close()

	children := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "children", "BACKEND-42", "--format", "json")
	children.assertExit(t, 0)
	childrenEnv := parseEnvelope(t, children.stdout)
	if childrenEnv["schema"] != "plane.work.children.v1" {
		t.Fatalf("unexpected children schema: %#v", childrenEnv)
	}
	if childrenEnv["data"].(map[string]any)["count"] != float64(1) {
		t.Fatalf("unexpected children payload: %#v", childrenEnv)
	}

	parent := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "parent", "set", "BACKEND-43", "BACKEND-42", "--apply", "--verify", "--format", "json")
	parent.assertExit(t, 0)
	parentEnv := parseEnvelope(t, parent.stdout)
	if parentEnv["schema"] != "plane.work.parent.set.v1" || parentEnv["data"].(map[string]any)["verified"] != true {
		t.Fatalf("unexpected parent set: %#v", parentEnv)
	}

	relation := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "relation", "add", "BACKEND-42", "--blocks", "BACKEND-43", "--apply", "--verify", "--format", "json")
	relation.assertExit(t, 0)
	relationEnv := parseEnvelope(t, relation.stdout)
	if relationEnv["schema"] != "plane.work.relation.add.v1" || relationEnv["data"].(map[string]any)["verified"] != true {
		t.Fatalf("unexpected relation add: %#v", relationEnv)
	}

	removed := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "relation", "remove", "BACKEND-42", "--blocks", "BACKEND-43", "--apply", "--verify", "--format", "json")
	removed.assertExit(t, 0)
	removedEnv := parseEnvelope(t, removed.stdout)
	if removedEnv["schema"] != "plane.work.relation.remove.v1" || removedEnv["data"].(map[string]any)["verified"] != true {
		t.Fatalf("unexpected relation remove: %#v", removedEnv)
	}
}

func TestIssue10SearchIncludeCommentsAndCreateDedupe(t *testing.T) {
	const apiKey = "issue10-search-secret"
	server := fakeIssue10RemainingPlane(t, apiKey)
	defer server.Close()

	search := runCLI(t, discoveryEnv(server.URL, apiKey), "search", "Investigate", "--project", "BACKEND", "--include-comments", "--max-results", "5", "--format", "json")
	search.assertExit(t, 0)
	searchEnv := parseEnvelope(t, search.stdout)
	results := searchEnv["data"].(map[string]any)["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("unexpected search results: %#v", searchEnv)
	}
	first := results[0].(map[string]any)
	if first["matched_field"] != "comment" || first["excerpt"] != "Investigate callback" {
		t.Fatalf("unexpected search match metadata: %#v", first)
	}

	create := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "create", "--project", "BACKEND", "--title", "Duplicate OAuth", "--dedupe-query", "OAuth", "--format", "json")
	create.assertExit(t, 0)
	createEnv := parseEnvelope(t, create.stdout)
	candidates := createEnv["data"].(map[string]any)["duplicate_candidates"].([]any)
	if len(candidates) == 0 {
		t.Fatalf("expected duplicate candidates: %#v", createEnv)
	}
}

func TestIssue10RateLimitErrorIsTyped(t *testing.T) {
	const apiKey = "issue10-rate-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Retry-After", "30")
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "project", "list", "--format", "json")
	res.assertExit(t, 1)
	env := parseEnvelope(t, res.stdout)
	errObj := env["error"].(map[string]any)
	if errObj["code"] != "RATE_LIMITED" || errObj["retryable"] != true || errObj["retry_after_seconds"] != float64(30) {
		t.Fatalf("unexpected rate limit error: %#v", errObj)
	}
}

type issue10RemainingState struct {
	lastPatch    map[string]any
	editPatches  int
	commentPosts int
	parentSet    bool
	relationSet  bool
}

func newIssue10RemainingState() *issue10RemainingState {
	return &issue10RemainingState{lastPatch: map[string]any{}}
}

func fakeIssue10RemainingPlane(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	return fakeIssue10RemainingPlaneWithState(t, apiKey, newIssue10RemainingState())
}

func fakeIssue10RemainingPlaneWithState(t *testing.T, apiKey string, state *issue10RemainingState) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		key := r.Method + " " + r.URL.Path
		switch key {
		case "GET /api/v1/workspaces/development/projects/":
			fmt.Fprint(w, `{"results":[{"id":"project-backend","identifier":"BACKEND","name":"Backend"}]}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/":
			fmt.Fprint(w, `{"results":[{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug with callback details</p>","state":"state-started","state_group":"started","priority":"medium","assignees":["member-bob"],"labels":["label-triage"]},{"id":"work-43","project":"project-backend","sequence_id":43,"name":"Add login metrics","description_html":"<p>Metrics details</p>","state":"state-ready","state_group":"started","priority":"low"}]}`)
		case "GET /api/v1/workspaces/development/work-items/BACKEND-42/":
			stateGroup := "started"
			stateID := "state-started"
			if state.lastPatch["state"] == "state-ready" {
				stateID = "state-ready"
			}
			if state.lastPatch["state_group"] != nil {
				stateGroup = fmt.Sprint(state.lastPatch["state_group"])
			}
			fmt.Fprintf(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"%s","state_group":"%s","priority":"%s","assignees":["member-alice"],"labels":["label-tracking"]}`, stateID, stateGroup, currentPriority(state))
		case "GET /api/v1/workspaces/development/work-items/BACKEND-43/":
			fmt.Fprint(w, `{"id":"work-43","project":"project-backend","sequence_id":43,"name":"Add login metrics","description_html":"<p>Metrics details</p>","state":"state-started","state_group":"started","priority":"low","parent":"work-42"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			fmt.Fprintf(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"%s","state_group":"started","priority":"%s","assignees":["member-alice"],"labels":["label-tracking"]}`, currentStateID(state), currentPriority(state))
		case "PATCH /api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			body := readJSONBody(t, r)
			state.lastPatch = body
			state.editPatches++
			fmt.Fprintf(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"%s","description_html":"%s","state":"%s","state_group":"started","priority":"%s","assignees":["member-alice"],"labels":["label-tracking"]}`, stringOrDefault(body["name"], "Fix OAuth"), stringOrDefault(body["description_html"], "<p>OAuth bug</p>"), currentStateID(state), currentPriority(state))
		case "PATCH /api/v1/workspaces/development/projects/project-backend/work-items/work-43/":
			body := readJSONBody(t, r)
			state.lastPatch = body
			if body["parent"] == "work-42" {
				state.parentSet = true
			}
			fmt.Fprint(w, `{"id":"work-43","project":"project-backend","sequence_id":43,"name":"Add login metrics","state":"state-started","state_group":"started","parent":"work-42"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/comments/":
			fmt.Fprint(w, `{"results":[{"id":"comment-1","comment_html":"<p>Investigate callback</p>","created_at":"2026-06-22T10:00:00Z"},{"id":"comment-2","comment_html":"<p>Second note</p>"}]}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-43/comments/":
			fmt.Fprint(w, `{"results":[]}`)
		case "POST /api/v1/workspaces/development/projects/project-backend/work-items/work-42/comments/":
			state.commentPosts++
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"comment-new","comment_html":"<p>Status: classified.</p>"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/states/":
			fmt.Fprint(w, `{"results":[{"id":"state-started","name":"In Progress","group":"started","default":true},{"id":"state-ready","name":"Ready","group":"started","default":false},{"id":"state-done","name":"Done","group":"completed","default":true}]}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/members/":
			fmt.Fprint(w, `{"results":[{"id":"member-alice","email":"alice@example.test","display_name":"Alice"},{"id":"member-bob","email":"bob@example.test","display_name":"Bob"}]}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/labels/":
			fmt.Fprint(w, `{"results":[{"id":"label-tracking","name":"tracking"},{"id":"label-triage","name":"needs-triage"}]}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/children/":
			fmt.Fprint(w, `{"results":[{"id":"work-43","project":"project-backend","sequence_id":43,"name":"Add login metrics","state":"state-started","state_group":"started"}]}`)
		case "POST /api/v1/workspaces/development/projects/project-backend/work-items/work-42/relations/":
			body := readJSONBody(t, r)
			if body["relation_type"] != "blocking" || body["related_work_item"] != "work-43" {
				t.Fatalf("unexpected relation body: %#v", body)
			}
			state.relationSet = true
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"relation-1","relation_type":"blocking","related_work_item":"work-43"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/relations/":
			if state.relationSet {
				fmt.Fprint(w, `{"results":[{"id":"relation-1","relation_type":"blocking","related_work_item":"work-43"}]}`)
			} else {
				fmt.Fprint(w, `{"results":[]}`)
			}
		case "DELETE /api/v1/workspaces/development/projects/project-backend/work-items/work-42/relations/relation-1/":
			state.relationSet = false
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
}

func currentPriority(state *issue10RemainingState) string {
	if p := stringOrDefault(state.lastPatch["priority"], ""); p != "" {
		return p
	}
	return "medium"
}

func currentStateID(state *issue10RemainingState) string {
	if s := stringOrDefault(state.lastPatch["state"], ""); s != "" {
		return s
	}
	return "state-started"
}

func stringOrDefault(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	return fmt.Sprint(value)
}

func assertStringContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("text did not contain %q: %s", want, text)
	}
}
