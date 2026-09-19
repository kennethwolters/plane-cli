package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestIssue11ProjectListBoundsOversizedProviderText(t *testing.T) {
	const apiKey = "bounded-project-secret"
	oversized := strings.Repeat("界", 100000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"results":[{"id":"project-large","identifier":"LARGE","name":%q,"description_text":%q}]}`, oversized, oversized)
	}))
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "project", "list", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	project := env["data"].(map[string]any)["projects"].([]any)[0].(map[string]any)
	name := project["name"].(string)
	if utf8.RuneCountInString(name) != 256 || !strings.HasSuffix(name, "…") {
		t.Fatalf("name is not deterministically rune-bounded: runes=%d suffix=%q", utf8.RuneCountInString(name), name[len(name)-3:])
	}
	if project["name_truncated"] != true {
		t.Fatalf("missing machine-readable truncation marker: %#v", project)
	}
	if _, present := project["description_text"]; present {
		t.Fatalf("project discovery should omit descriptions: %#v", project)
	}
	if len(res.stdout) > 5000 || strings.Contains(res.stdout, strings.Repeat("界", 1000)) {
		t.Fatalf("serialized discovery output is not bounded: %d bytes", len(res.stdout))
	}

	textRes := runCLI(t, discoveryEnv(server.URL, apiKey), "project", "list")
	textRes.assertExit(t, 0)
	if utf8.RuneCountInString(textRes.stdout) > 320 || !strings.Contains(textRes.stdout, "…") {
		t.Fatalf("terminal discovery output is not bounded: %d runes", utf8.RuneCountInString(textRes.stdout))
	}
}

func TestIssue11WorkGetUsesBoundedPlainTextWithoutEditorPayload(t *testing.T) {
	const apiKey = "bounded-work-secret"
	oversized := strings.Repeat("payload ", 20000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces/development/work-items/BACKEND-42/":
			fmt.Fprintf(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Bound output","description_html":%q,"description_binary":"must-not-leak","state_group":"started"}`, "<p>"+oversized+"</p>")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "get", "BACKEND-42", "--format", "json")
	res.assertExit(t, 0)
	if strings.Contains(res.stdout, "description_html") || strings.Contains(res.stdout, "description_binary") || strings.Contains(res.stdout, "must-not-leak") {
		t.Fatalf("work get leaked provider editor payload: %s", res.stdout)
	}
	env := parseEnvelope(t, res.stdout)
	item := env["data"].(map[string]any)["work_item"].(map[string]any)
	description := item["description_text"].(string)
	if utf8.RuneCountInString(description) != 4096 || !strings.HasSuffix(description, "…") || item["description_text_truncated"] != true {
		t.Fatalf("description is not deterministically bounded: %#v", item)
	}
	if item["description_text_original_runes"].(float64) <= 4096 {
		t.Fatalf("missing original size metadata: %#v", item)
	}
	if len(res.stdout) > 10000 {
		t.Fatalf("serialized detail output is not bounded: %d bytes", len(res.stdout))
	}
}

func TestIssue11WorkListAndSearchStayCompact(t *testing.T) {
	const apiKey = "bounded-search-secret"
	oversizedName := strings.Repeat("N", 10000)
	oversizedDescription := "<p>needle " + strings.Repeat("payload ", 20000) + "</p>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces/development/projects/":
			fmt.Fprint(w, `{"results":[{"id":"project-backend","identifier":"BACKEND","name":"Backend"}]}`)
		case "/api/v1/workspaces/development/projects/project-backend/work-items/":
			fmt.Fprintf(w, `{"results":[{"id":"work-42","project":"project-backend","sequence_id":42,"name":%q,"description_html":%q,"state_group":"started"}]}`, oversizedName, oversizedDescription)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	listRes := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "list", "--project", "BACKEND", "--format", "json")
	listRes.assertExit(t, 0)
	if strings.Contains(listRes.stdout, "description_html") || strings.Contains(listRes.stdout, "description_text") || len(listRes.stdout) > 6000 {
		t.Fatalf("work list emitted heavyweight content: %d bytes", len(listRes.stdout))
	}
	listEnv := parseEnvelope(t, listRes.stdout)
	item := listEnv["data"].(map[string]any)["work_items"].([]any)[0].(map[string]any)
	if item["name_truncated"] != true || utf8.RuneCountInString(item["name"].(string)) != 256 {
		t.Fatalf("work list name is not bounded: %#v", item)
	}

	searchRes := runCLI(t, discoveryEnv(server.URL, apiKey), "search", "needle", "--project", "BACKEND", "--format", "json")
	searchRes.assertExit(t, 0)
	if strings.Contains(searchRes.stdout, "description_html") || len(searchRes.stdout) > 6000 {
		t.Fatalf("search emitted heavyweight content: %d bytes", len(searchRes.stdout))
	}
	searchEnv := parseEnvelope(t, searchRes.stdout)
	result := searchEnv["data"].(map[string]any)["results"].([]any)[0].(map[string]any)
	if _, present := result["description_text"]; present {
		t.Fatalf("search should emit only the matched excerpt: %#v", result)
	}
	if result["name_truncated"] != true || !strings.HasSuffix(result["excerpt"].(string), "…") {
		t.Fatalf("search result is not compact and explicit: %#v", result)
	}
}

func TestIssue11MutationDryRunSummarizesEditorPayload(t *testing.T) {
	const apiKey = "bounded-mutation-secret"
	server := fakeWorkPlane(t, apiKey)
	defer server.Close()
	body := "<p>" + strings.Repeat("change ", 10000) + "</p>"

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "edit", "BACKEND-42", "--description-html", body, "--format", "json")
	res.assertExit(t, 0)
	if strings.Contains(res.stdout, "description_html") || strings.Contains(res.stdout, strings.Repeat("change ", 1000)) {
		t.Fatalf("dry-run echoed an unbounded editor payload: %d bytes", len(res.stdout))
	}
	env := parseEnvelope(t, res.stdout)
	changes := env["data"].(map[string]any)["operation"].(map[string]any)["changes"].(map[string]any)
	if changes["description_text_truncated"] != true || changes["description_text_original_runes"].(float64) <= 4096 {
		t.Fatalf("dry-run summary lacks truncation metadata: %#v", changes)
	}
}

func TestIssue11CommentsMapAlternateBodiesToBoundedPlainText(t *testing.T) {
	const apiKey = "bounded-comments-secret"
	oversized := "<p>" + strings.Repeat("comment ", 20000) + "</p>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces/development/work-items/BACKEND-42/":
			fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Comments","state_group":"started"}`)
		case "/api/v1/workspaces/development/projects/project-backend/work-items/work-42/comments/":
			fmt.Fprintf(w, `{"results":[{"id":"comment-1","body":%q,"editor":{"binary":"must-not-leak"},"content":"must-not-leak"}]}`, oversized)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "comments", "BACKEND-42", "--format", "json")
	res.assertExit(t, 0)
	if strings.Contains(res.stdout, "must-not-leak") || strings.Contains(res.stdout, "comment_html") || strings.Contains(res.stdout, `"body"`) {
		t.Fatalf("comments leaked alternate editor fields: %s", res.stdout)
	}
	env := parseEnvelope(t, res.stdout)
	comment := env["data"].(map[string]any)["comments"].([]any)[0].(map[string]any)
	if comment["comment_text_truncated"] != true || utf8.RuneCountInString(comment["comment_text"].(string)) != 4096 {
		t.Fatalf("comment body is not bounded: %#v", comment)
	}
}

func TestIssue11ApplyPreservesExactMutationPayload(t *testing.T) {
	const apiKey = "exact-mutation-secret"
	body := "<p>literal &lt;tag&gt; " + strings.Repeat("界", 5000) + "</p>"
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workspaces/development/work-items/BACKEND-42/":
			fmt.Fprint(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Exact payload","description_html":"<p>old</p>","state_group":"started"}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/workspaces/development/projects/project-backend/work-items/work-42/":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			received, _ = payload["description_html"].(string)
			fmt.Fprintf(w, `{"id":"work-42","project":"project-backend","sequence_id":42,"name":"Exact payload","description_html":%q,"state_group":"started"}`, received)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "work", "edit", "BACKEND-42", "--description-html", body, "--apply", "--format", "json")
	res.assertExit(t, 0)
	if received != body {
		t.Fatalf("mutation payload changed: got %d bytes, want %d", len(received), len(body))
	}
	if strings.Contains(res.stdout, strings.Repeat("界", 4500)) || strings.Contains(res.stdout, "description_html") {
		t.Fatalf("mutation result echoed the full editor payload: %d bytes", len(res.stdout))
	}
}

func TestIssue11PreservesMachineIdentifiersAndURLs(t *testing.T) {
	longID := strings.Repeat("i", 600)
	longURL := "https://example.test/" + strings.Repeat("segment/", 100)
	longSlug := strings.Repeat("workspace-", 80)
	sanitized := sanitizeAgentOutput(map[string]any{
		"id":             longID,
		"permalink":      longURL,
		"workspace_slug": longSlug,
		"name":           strings.Repeat("n", 600),
	}).(map[string]any)
	if sanitized["id"] != longID || sanitized["permalink"] != longURL || sanitized["workspace_slug"] != longSlug {
		t.Fatalf("machine-readable identifiers were changed: %#v", sanitized)
	}
	if sanitized["name_truncated"] != true {
		t.Fatalf("human-readable name was not bounded: %#v", sanitized)
	}
}

func TestIssue11RejectsOversizedRequestedExcerpt(t *testing.T) {
	res := runCLI(t, nil, "work", "list", "--project", "BACKEND", "--description-excerpt", "4097", "--format", "json")
	res.assertExit(t, 1)
	assertErrorCode(t, res.stdout, "VALIDATION_FAILED")
}
