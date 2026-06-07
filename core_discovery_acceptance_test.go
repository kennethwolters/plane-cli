package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCoreProjectListJSONContract(t *testing.T) {
	const apiKey = "project-list-secret"
	server := fakeDiscoveryPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "project", "list", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.project.list.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	projects := data["projects"].([]any)
	if len(projects) != 2 {
		t.Fatalf("project count = %d, want 2", len(projects))
	}
	first := projects[0].(map[string]any)
	if first["identifier"] != "BACKEND" || first["id"] != "project-backend" {
		t.Fatalf("unexpected project: %#v", first)
	}
}

func TestCoreProjectGetResolvesIdentifier(t *testing.T) {
	const apiKey = "project-get-secret"
	server := fakeDiscoveryPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "project", "get", "BACKEND", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.project.get.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	project := env["data"].(map[string]any)["project"].(map[string]any)
	if project["id"] != "project-backend" || project["name"] != "Backend" {
		t.Fatalf("unexpected project: %#v", project)
	}
}

func TestCoreStateListForProject(t *testing.T) {
	const apiKey = "state-list-secret"
	server := fakeDiscoveryPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "state", "list", "--project", "BACKEND", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.state.list.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	states := data["states"].([]any)
	if len(states) != 2 {
		t.Fatalf("state count = %d, want 2", len(states))
	}
	if states[1].(map[string]any)["group"] != "completed" {
		t.Fatalf("unexpected completed state: %#v", states[1])
	}
}

func TestCoreMemberListForProject(t *testing.T) {
	const apiKey = "member-list-secret"
	server := fakeDiscoveryPlane(t, apiKey)
	defer server.Close()

	res := runCLI(t, discoveryEnv(server.URL, apiKey), "member", "list", "--project", "BACKEND", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.member.list.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	members := env["data"].(map[string]any)["members"].([]any)
	if len(members) != 1 || members[0].(map[string]any)["email"] != "agent@example.test" {
		t.Fatalf("unexpected members: %#v", members)
	}
}

func TestCoreStateListRequiresProject(t *testing.T) {
	res := runCLI(t, nil, "state", "list", "--format", "json")
	res.assertExit(t, 1)
	assertErrorCode(t, res.stdout, "MISSING_PROJECT_REFERENCE")
}

func discoveryEnv(serverURL, apiKey string) map[string]string {
	return map[string]string{
		"PLANE_API_KEY":        apiKey,
		"PLANE_BASE_URL":       serverURL,
		"PLANE_WORKSPACE_SLUG": "development",
	}
}

func fakeDiscoveryPlane(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workspaces/development/projects/":
			fmt.Fprint(w, `{"results":[{"id":"project-backend","identifier":"BACKEND","name":"Backend","description_text":"Backend services"},{"id":"project-web","identifier":"WEB","name":"Web"}]}`)
		case "/api/v1/workspaces/development/projects/project-backend/states/":
			fmt.Fprint(w, `{"results":[{"id":"state-open","name":"Open","group":"started","color":"#111111","default":true,"slug":"open"},{"id":"state-done","name":"Done","group":"completed","color":"#00ff00","default":false,"slug":"done"}]}`)
		case "/api/v1/workspaces/development/projects/project-backend/members/":
			fmt.Fprint(w, `[{"id":"user-1","email":"agent@example.test","display_name":"Agent"}]`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
}
