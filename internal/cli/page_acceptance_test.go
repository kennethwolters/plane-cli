package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageListReturnsBoundedProjectPageMetadata(t *testing.T) {
	const apiKey = "page-list-secret"
	server := fakePagesPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "page", "list", "--project", "AEDFU", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.page.list.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	pages := env["data"].(map[string]any)["pages"].([]any)
	if len(pages) != 2 {
		t.Fatalf("page count = %d, want 2", len(pages))
	}
	first := pages[0].(map[string]any)
	if first["id"] != "page-guardrails" || first["name"] != "Guardrails" {
		t.Fatalf("unexpected page: %#v", first)
	}
	second := pages[1].(map[string]any)
	if second["name_truncated"] != true || len([]rune(second["name"].(string))) != maxOutputMetadataRunes || !strings.HasSuffix(second["name"].(string), "…") {
		t.Fatalf("oversized page name was not bounded: %#v", second)
	}
	if strings.Contains(res.stdout, "description_binary") || strings.Contains(res.stdout, "description_html") {
		t.Fatalf("page list leaked heavy content: %s", res.stdout)
	}
}

func TestPageGetReturnsPlainTextWithoutEditorPayloads(t *testing.T) {
	const apiKey = "page-get-secret"
	server := fakePagesPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "page", "get", "page-guardrails", "--project", "AEDFU", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.page.get.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	page := env["data"].(map[string]any)["page"].(map[string]any)
	if page["description_text"] != "Read this first." {
		t.Fatalf("unexpected page content: %#v", page)
	}
	if strings.Contains(res.stdout, "secret-binary") || strings.Contains(res.stdout, "editor-paragraph") {
		t.Fatalf("page get leaked provider editor payloads: %s", res.stdout)
	}
}

func TestPageGetBoundsOversizedUnicodePlainText(t *testing.T) {
	const apiKey = "page-detail-bound-secret"
	oversized := strings.Repeat("界", maxOutputDetailRunes+100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces/development/projects/":
			fmt.Fprint(w, `{"results":[{"id":"project-aedfu","identifier":"AEDFU","name":"Functional Solution"}]}`)
		case "/api/v1/workspaces/development/projects/project-aedfu/pages/":
			fmt.Fprint(w, `{"results":[{"id":"page-large","name":"Large"}]}`)
		case "/api/v1/workspaces/development/projects/project-aedfu/pages/page-large/":
			fmt.Fprintf(w, `{"id":"page-large","name":"Large","description_stripped":%q,"description_html":"must-not-leak","description_binary":"must-not-leak"}`, oversized)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "page", "get", "page-large", "--project", "AEDFU", "--format", "json")
	res.assertExit(t, 0)
	if strings.Contains(res.stdout, "must-not-leak") || strings.Contains(res.stdout, "description_html") {
		t.Fatalf("page get leaked editor payloads: %s", res.stdout)
	}
	env := parseEnvelope(t, res.stdout)
	page := env["data"].(map[string]any)["page"].(map[string]any)
	if page["description_text_truncated"] != true || len([]rune(page["description_text"].(string))) != maxOutputDetailRunes || !strings.HasSuffix(page["description_text"].(string), "…") {
		t.Fatalf("oversized page detail was not bounded: %#v", page)
	}
}

func TestPageListRequiresProject(t *testing.T) {
	res := runCLI(t, nil, "page", "list", "--format", "json")
	res.assertExit(t, 1)
	assertErrorCode(t, res.stdout, "MISSING_PROJECT_REFERENCE")
}

func fakePagesPlane(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces/development/projects/":
			fmt.Fprint(w, `{"results":[{"id":"project-aedfu","identifier":"AEDFU","name":"Functional Solution"}]}`)
		case "/api/v1/workspaces/development/projects/project-aedfu/pages/":
			fmt.Fprintf(w, `{"results":[{"id":"page-guardrails","name":"Guardrails","updated_at":"2026-09-19T00:00:00Z"},{"id":"page-large","name":%q}]}`, strings.Repeat("x", maxOutputMetadataRunes+50))
		case "/api/v1/workspaces/development/projects/project-aedfu/pages/page-guardrails/":
			fmt.Fprint(w, `{"id":"page-guardrails","name":"Guardrails","description_stripped":"Read this first.","description_html":"<p class=\"editor-paragraph\">Read this first.</p>","description_binary":"secret-binary","updated_at":"2026-09-19T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
}
