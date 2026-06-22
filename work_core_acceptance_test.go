package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreWorkListJSONContract(t *testing.T) {
	const apiKey = "work-list-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "list", "--project", "BACKEND", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.list.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	items := env["data"].(map[string]any)["work_items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["readable_id"] != "BACKEND-42" {
		t.Fatalf("unexpected work items: %#v", items)
	}
}

func TestCoreWorkGetJSONContract(t *testing.T) {
	const apiKey = "work-get-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "get", "BACKEND-42", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.get.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	item := env["data"].(map[string]any)["work_item"].(map[string]any)
	if item["work_item_id"] != "work-42" || item["name"] != "Fix OAuth" {
		t.Fatalf("unexpected work item: %#v", item)
	}
}

func TestCoreWorkCreateDryRunDoesNotMutate(t *testing.T) {
	const apiKey = "work-create-dry-run-secret"
	created := false
	server := fakeWorkPlaneWithCreateHook(t, apiKey, func() { created = true })
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "create", "--project", "BACKEND", "--title", "New task", "--format", "json")
	res.assertExit(t, 0)
	if created {
		t.Fatal("dry-run work create unexpectedly called POST")
	}
	env := parseEnvelope(t, res.stdout)
	data := env["data"].(map[string]any)
	if data["applied"] != false || data["verified"] != false {
		t.Fatalf("unexpected dry-run data: %#v", data)
	}
}

func TestCoreWorkCreateApplyVerify(t *testing.T) {
	const apiKey = "work-create-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "create", "--project", "BACKEND", "--title", "New task", "--apply", "--verify", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.create.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["applied"] != true || data["verified"] != true {
		t.Fatalf("create was not applied and verified: %#v", data)
	}
}

func TestCoreWorkCommentApply(t *testing.T) {
	const apiKey = "work-comment-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "comment", "BACKEND-42", "--html", "<p>Looks good</p>", "--apply", "--verify", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.comment.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["applied"] != true || data["verified"] != true {
		t.Fatalf("comment was not applied and verified: %#v", data)
	}
}

func TestIssue10WorkFileInputsDryRunDoesNotMutate(t *testing.T) {
	cases := []struct {
		name         string
		args         func(string) []string
		schema       string
		changedField string
	}{
		{
			name: "create description file",
			args: func(path string) []string {
				return []string{"work", "create", "--project", "BACKEND", "--title", "New task", "--description-file", path, "--format", "json"}
			},
			schema:       "plane.work.create.v1",
			changedField: "description_html",
		},
		{
			name: "edit description file",
			args: func(path string) []string {
				return []string{"work", "edit", "BACKEND-42", "--description-file", path, "--format", "json"}
			},
			schema:       "plane.work.edit.v1",
			changedField: "description_html",
		},
		{
			name: "comment html file",
			args: func(path string) []string {
				return []string{"work", "comment", "BACKEND-42", "--html-file", path, "--format", "json"}
			},
			schema:       "plane.work.comment.v1",
			changedField: "comment_html",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const apiKey = "work-file-dry-run-secret"
			mutations := issue10MutationCounters{}
			server := fakeIssue10WorkPlane(t, apiKey, &mutations)
			defer server.Close()
			bodyPath := writeTempBodyFile(t, "<p>From file</p>")

			res := runCLI(t, discoveryEnv(server.URL, apiKey), tc.args(bodyPath)...)
			res.assertExit(t, 0)
			env := parseEnvelope(t, res.stdout)
			if env["schema"] != tc.schema {
				t.Fatalf("unexpected schema: %#v", env)
			}
			data := env["data"].(map[string]any)
			if data["applied"] != false || data["verified"] != false {
				t.Fatalf("unexpected mutation state: %#v", data)
			}
			changes := data["operation"].(map[string]any)["changes"].(map[string]any)
			if changes[tc.changedField] != "<p>From file</p>" {
				t.Fatalf("file body was not reflected in operation: %#v", changes)
			}
			if mutations.createPosts != 0 || mutations.editPatches != 0 || mutations.commentPosts != 0 {
				t.Fatalf("dry-run mutated server: %#v", mutations)
			}
		})
	}
}

func TestIssue10WorkFileInputsRejectMissingFiles(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "create description file",
			args: []string{"work", "create", "--project", "BACKEND", "--title", "New task", "--description-file", filepath.Join(t.TempDir(), "missing-create.html"), "--format", "json"},
		},
		{
			name: "edit description file",
			args: []string{"work", "edit", "BACKEND-42", "--description-file", filepath.Join(t.TempDir(), "missing-edit.html"), "--format", "json"},
		},
		{
			name: "comment html file",
			args: []string{"work", "comment", "BACKEND-42", "--html-file", filepath.Join(t.TempDir(), "missing-comment.html"), "--format", "json"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runCLI(t, nil, tc.args...)
			res.assertExit(t, 1)
			assertErrorCode(t, res.stdout, "FILE_NOT_FOUND")
		})
	}
}

func TestIssue10WorkFileInputsRejectInlineAndFileFlags(t *testing.T) {
	bodyPath := writeTempBodyFile(t, "<p>From file</p>")
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "create description",
			args: []string{"work", "create", "--project", "BACKEND", "--title", "New task", "--description-html", "<p>Inline</p>", "--description-file", bodyPath, "--format", "json"},
		},
		{
			name: "edit description",
			args: []string{"work", "edit", "BACKEND-42", "--description-html", "<p>Inline</p>", "--description-file", bodyPath, "--format", "json"},
		},
		{
			name: "comment html",
			args: []string{"work", "comment", "BACKEND-42", "--html", "<p>Inline</p>", "--html-file", bodyPath, "--format", "json"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runCLI(t, nil, tc.args...)
			res.assertExit(t, 1)
			assertErrorCode(t, res.stdout, "VALIDATION_FAILED")
		})
	}
}

func TestIssue10WorkEditVerifyChecksDescriptionHTML(t *testing.T) {
	const apiKey = "work-edit-verify-description-secret"
	mutations := issue10MutationCounters{}
	server := fakeIssue10WorkPlane(t, apiKey, &mutations)
	defer server.Close()
	bodyPath := writeTempBodyFile(t, "<p>From file</p>")

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "edit", "BACKEND-42", "--description-file", bodyPath, "--apply", "--verify", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.edit.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["applied"] != true {
		t.Fatalf("edit was not applied: %#v", data)
	}
	if data["verified"] != false {
		t.Fatalf("stale description should not verify: %#v", data)
	}
	if mutations.editPatches != 1 {
		t.Fatalf("expected one edit patch, got %#v", mutations)
	}
}

func TestIssue10WorkCommentsListJSONContract(t *testing.T) {
	const apiKey = "work-comments-secret"
	mutations := issue10MutationCounters{}
	server := fakeIssue10WorkPlane(t, apiKey, &mutations)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "comments", "BACKEND-42", "--limit", "2", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.comments.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["count"] != float64(2) {
		t.Fatalf("unexpected comment count: %#v", data)
	}
	workItem := data["work_item"].(map[string]any)
	if workItem["readable_id"] != "BACKEND-42" || workItem["work_item_id"] != "work-42" {
		t.Fatalf("unexpected work item: %#v", workItem)
	}
	comments := data["comments"].([]any)
	if len(comments) != 2 {
		t.Fatalf("unexpected comments: %#v", comments)
	}
	first := comments[0].(map[string]any)
	if first["id"] != "comment-1" || first["comment_html"] != "<p>First</p>" {
		t.Fatalf("unexpected first comment: %#v", first)
	}
}

func TestCoreWorkCompleteUsesStateGroupAndEvidence(t *testing.T) {
	const apiKey = "work-complete-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "complete", "BACKEND-42", "--evidence", "Tests passed", "--apply", "--verify", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.work.complete.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["applied"] != true || data["verified"] != true {
		t.Fatalf("complete was not applied and verified: %#v", data)
	}
}

func TestCoreWorkLifecycleDryRunPlansStateGroups(t *testing.T) {
	cases := []struct {
		action string
		args   []string
		group  string
	}{
		{action: "start", args: []string{"--reason", "begin"}, group: "started"},
		{action: "reopen", args: []string{"--reason", "needs follow-up"}, group: "started"},
		{action: "cancel", args: []string{"--reason", "not needed"}, group: "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			const apiKey = "work-lifecycle-dry-run-secret"
			server := fakeWorkPlane(t, apiKey)
			defer server.Close()
			args := append([]string{"work", tc.action, "BACKEND-42"}, tc.args...)
			args = append(args, "--format", "json")
			res := runCLI(t, discoveryEnv(server.URL, apiKey), args...)
			res.assertExit(t, 0)
			env := parseEnvelope(t, res.stdout)
			if env["schema"] != "plane.work."+tc.action+".v1" {
				t.Fatalf("unexpected schema: %#v", env)
			}
			data := env["data"].(map[string]any)
			operation := data["operation"].(map[string]any)
			changes := operation["changes"].(map[string]any)
			if data["applied"] != false || changes["state_group"] != tc.group {
				t.Fatalf("unexpected dry-run operation: %#v", data)
			}
		})
	}
}

func TestCoreSearchFindsWorkItems(t *testing.T) {
	const apiKey = "search-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "search", "OAuth", "--project", "BACKEND", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.search.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	results := env["data"].(map[string]any)["results"].([]any)
	if len(results) != 1 || results[0].(map[string]any)["readable_id"] != "BACKEND-42" {
		t.Fatalf("unexpected search results: %#v", results)
	}
}

func fakeWorkPlane(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	return fakeWorkPlaneWithCreateHook(t, apiKey, func() {})
}

func fakeWorkPlaneWithCreateHook(t *testing.T, apiKey string, onCreate func()) *httptest.Server {
	t.Helper()
	completed := false
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
			fmt.Fprint(w, `{"results":[{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"state-started","state_group":"started"}]}`)
		case "GET /api/v1/workspaces/development/work-items/BACKEND-42/":
			if completed {
				fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"state-done","state_group":"completed"}`)
			} else {
				fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"state-started","state_group":"started"}`)
			}
		case "GET /api/v1/workspaces/development/work-items/BACKEND-43/":
			fmt.Fprint(w, `{"id":"work-43","project":"project-backend","sequence_id":43,"name":"New task","state":"state-started","state_group":"started"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			if completed {
				fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","state":"state-done","state_group":"completed"}`)
			} else {
				fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","state":"state-started","state_group":"started"}`)
			}
		case "POST /api/v1/workspaces/development/projects/project-backend/work-items/":
			onCreate()
			body := readJSONBody(t, r)
			if body["name"] != "New task" {
				t.Fatalf("unexpected create body: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"work-43","project":"project-backend","sequence_id":43,"name":"New task","state":"state-started","state_group":"started"}`)
		case "POST /api/v1/workspaces/development/projects/project-backend/work-items/work-42/comments/":
			body := readJSONBody(t, r)
			if !strings.Contains(fmt.Sprint(body["comment_html"]), "Looks good") && !strings.Contains(fmt.Sprint(body["comment_html"]), "Tests passed") {
				t.Fatalf("unexpected comment body: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"comment-1","comment_html":"<p>ok</p>"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/states/":
			fmt.Fprint(w, `{"results":[{"id":"state-started","name":"In Progress","group":"started","default":true},{"id":"state-done","name":"Done","group":"completed","default":true},{"id":"state-cancelled","name":"Cancelled","group":"cancelled","default":true}]}`)
		case "PATCH /api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			body := readJSONBody(t, r)
			if body["state"] != "state-done" {
				t.Fatalf("unexpected patch body: %#v", body)
			}
			completed = true
			fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","state":"state-done","state_group":"completed"}`)
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
}

type issue10MutationCounters struct {
	createPosts  int
	editPatches  int
	commentPosts int
}

func fakeIssue10WorkPlane(t *testing.T, apiKey string, mutations *issue10MutationCounters) *httptest.Server {
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
		case "GET /api/v1/workspaces/development/work-items/BACKEND-42/":
			fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"state-started","state_group":"started"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>OAuth bug</p>","state":"state-started","state_group":"started"}`)
		case "POST /api/v1/workspaces/development/projects/project-backend/work-items/":
			mutations.createPosts++
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"work-43","project":"project-backend","sequence_id":43,"name":"New task","description_html":"<p>From file</p>","state":"state-started","state_group":"started"}`)
		case "PATCH /api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			mutations.editPatches++
			fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Fix OAuth","description_html":"<p>From file</p>","state":"state-started","state_group":"started"}`)
		case "POST /api/v1/workspaces/development/projects/project-backend/work-items/work-42/comments/":
			mutations.commentPosts++
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"comment-1","comment_html":"<p>From file</p>"}`)
		case "GET /api/v1/workspaces/development/projects/project-backend/work-items/work-42/comments/":
			fmt.Fprint(w, `{"results":[{"id":"comment-1","comment_html":"<p>First</p>"},{"id":"comment-2","comment_html":"<p>Second</p>"},{"id":"comment-3","comment_html":"<p>Third</p>"}]}`)
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
}

func writeTempBodyFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body.html")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	return body
}
