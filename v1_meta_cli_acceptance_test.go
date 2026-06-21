package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var testBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "plane-cli-acceptance-")
	if err != nil {
		panic(err)
	}
	testBinary = filepath.Join(dir, "plane-cli")
	cmd := exec.Command("go", "build", "-o", testBinary, ".")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestV1VersionJSONContract(t *testing.T) {
	res := runCLI(t, nil, "version", "--format", "json")
	res.assertExit(t, 0)
	env := parseEnvelope(t, res.stdout)
	if env["ok"] != true || env["schema"] != "plane.version.v1" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["version"] == "" || len(data["supported_schemas"].([]any)) == 0 {
		t.Fatalf("missing version data: %#v", data)
	}
}

func TestV1ConfigGetRedactsSecrets(t *testing.T) {
	secret := "super-secret-api-key"
	res := runCLI(t, map[string]string{
		"PLANE_API_KEY":        secret,
		"PLANE_BASE_URL":       "https://plane.example.test",
		"PLANE_WORKSPACE_SLUG": "development",
	}, "config", "get", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, secret)
	env := parseEnvelope(t, res.stdout)
	data := env["data"].(map[string]any)
	apiKey := data["api_key"].(map[string]any)
	if apiKey["present"] != true || apiKey["value"] != nil {
		t.Fatalf("api key should be present but not valued: %#v", apiKey)
	}
}

func TestV1ConfigSetRejectsSecretKeys(t *testing.T) {
	res := runCLI(t, nil, "config", "set", "api_key", "super-secret", "--format", "json")
	res.assertExit(t, 1)
	res.assertNoSecret(t, "super-secret")
	assertErrorCode(t, res.stdout, "CONFIG_WRITE_REJECTED_SECRET")
}

func TestV1AuthStatusMissingConfigIsTypedAndOffline(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatal("auth status should not call network when required config is missing")
	}))
	defer server.Close()
	res := runCLI(t, map[string]string{"PLANE_BASE_URL": server.URL}, "auth", "status", "--format", "json")
	res.assertExit(t, 1)
	assertErrorCode(t, res.stdout, "MISSING_API_KEY")
	if called {
		t.Fatal("unexpected network call")
	}
}

func TestV1AuthStatusValidatesPATWithFakePlane(t *testing.T) {
	const apiKey = "test-api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/me/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"user-1","email":"agent@example.test","display_name":"Agent"}`)
	}))
	defer server.Close()
	res := runCLI(t, map[string]string{
		"PLANE_API_KEY":        apiKey,
		"PLANE_BASE_URL":       server.URL,
		"PLANE_WORKSPACE_SLUG": "development",
	}, "auth", "status", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.auth.status.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
}

func TestV1DoctorForAgentIsActionable(t *testing.T) {
	const apiKey = "doctor-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"user-1","email":"agent@example.test"}`)
	}))
	defer server.Close()
	res := runCLI(t, map[string]string{
		"PLANE_API_KEY":        apiKey,
		"PLANE_BASE_URL":       server.URL,
		"PLANE_WORKSPACE_SLUG": "development",
	}, "doctor", "--for-agent", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	if env["schema"] != "plane.doctor.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	if data["for_agent"] != true || len(data["checks"].([]any)) == 0 {
		t.Fatalf("doctor output is not actionable: %#v", data)
	}
}

func TestV1ResolveReadableWorkItemID(t *testing.T) {
	const apiKey = "resolve-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspaces/development/work-items/ENG-42/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"work-uuid","project_id":"project-uuid","name":"OAuth bug"}`)
	}))
	defer server.Close()
	res := runCLI(t, map[string]string{
		"PLANE_API_KEY":        apiKey,
		"PLANE_BASE_URL":       server.URL,
		"PLANE_WORKSPACE_SLUG": "development",
	}, "resolve", "ENG-42", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	data := env["data"].(map[string]any)
	if data["readable_id"] != "ENG-42" || data["work_item_id"] != "work-uuid" || data["project_id"] != "project-uuid" {
		t.Fatalf("unexpected resolve data: %#v", data)
	}
}

func TestV1ResolveInvalidReferenceIsTyped(t *testing.T) {
	res := runCLI(t, nil, "resolve", "not-a-plane-ref", "--format", "json")
	res.assertExit(t, 1)
	assertErrorCode(t, res.stdout, "INVALID_REFERENCE")
}

type cliResult struct {
	stdout string
	stderr string
	code   int
}

func runCLI(t *testing.T, extraEnv map[string]string, args ...string) cliResult {
	t.Helper()
	return runCLIInDir(t, t.TempDir(), extraEnv, args...)
}

func runCLIInDir(t *testing.T, workDir string, extraEnv map[string]string, args ...string) cliResult {
	t.Helper()
	home := t.TempDir()
	xdg := filepath.Join(home, "xdg")
	cmd := exec.Command(testBinary, args...)
	cmd.Dir = workDir
	cmd.Env = cleanEnv(os.Environ())
	cmd.Env = append(cmd.Env, "HOME="+home, "XDG_CONFIG_HOME="+xdg)
	for key, value := range extraEnv {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed to start: %v", err)
		}
	}
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func writeTestEnvFile(t *testing.T, path string, values map[string]string) {
	t.Helper()
	var b strings.Builder
	for key, value := range values {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(value)
		b.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func cleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "PLANE_") || strings.HasPrefix(entry, "XDG_CONFIG_HOME=") || strings.HasPrefix(entry, "HOME=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (r cliResult) assertExit(t *testing.T, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("exit code %d, want %d\nstdout:%s\nstderr:%s", r.code, want, r.stdout, r.stderr)
	}
}

func (r cliResult) assertNoSecret(t *testing.T, secret string) {
	t.Helper()
	if strings.Contains(r.stdout, secret) || strings.Contains(r.stderr, secret) {
		t.Fatalf("secret leaked\nstdout:%s\nstderr:%s", r.stdout, r.stderr)
	}
}

func parseEnvelope(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	return env
}

func assertErrorCode(t *testing.T, stdout, want string) {
	t.Helper()
	env := parseEnvelope(t, stdout)
	if env["ok"] != false || env["schema"] != "plane.error.v1" {
		t.Fatalf("not an error envelope: %#v", env)
	}
	errObj := env["error"].(map[string]any)
	if errObj["code"] != want {
		t.Fatalf("error code %v, want %s", errObj["code"], want)
	}
}
