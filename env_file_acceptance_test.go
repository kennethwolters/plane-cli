package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvFileFlagAuthStatus(t *testing.T) {
	const apiKey = "flag-env-file-secret"
	server := fakeAuthPlane(t, apiKey)
	defer server.Close()

	workDir := t.TempDir()
	envPath := filepath.Join(workDir, "plane.env")
	writeTestEnvFile(t, envPath, discoveryEnv(server.URL, apiKey))

	res := runCLIInDir(t, workDir, nil, "--env-file", envPath, "auth", "status", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
}

func TestEnvFileEnvVarAuthStatus(t *testing.T) {
	const apiKey = "env-var-env-file-secret"
	server := fakeAuthPlane(t, apiKey)
	defer server.Close()

	workDir := t.TempDir()
	envPath := filepath.Join(workDir, ".plane.env")
	writeTestEnvFile(t, envPath, discoveryEnv(server.URL, apiKey))

	res := runCLIInDir(t, workDir, map[string]string{"PLANE_CLI_ENV_FILE": envPath}, "auth", "status", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
}

func TestAncestorDotenvDiscovery(t *testing.T) {
	const apiKey = "ancestor-dotenv-secret"
	server := fakeAuthPlane(t, apiKey)
	defer server.Close()

	root := t.TempDir()
	nested := filepath.Join(root, "packages", "cli")
	writeTestEnvFile(t, filepath.Join(root, ".env"), discoveryEnv(server.URL, apiKey))
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	res := runCLIInDir(t, nested, nil, "auth", "status", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
}

func TestProcessEnvOverridesEnvFile(t *testing.T) {
	const goodKey = "process-env-secret"
	server := fakeAuthPlane(t, goodKey)
	defer server.Close()

	workDir := t.TempDir()
	envPath := filepath.Join(workDir, ".env")
	writeTestEnvFile(t, envPath, discoveryEnv(server.URL, "wrong-env-file-secret"))

	res := runCLIInDir(t, workDir, map[string]string{"PLANE_API_KEY": goodKey}, "auth", "status", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, goodKey)
	res.assertNoSecret(t, "wrong-env-file-secret")
}

func TestDoctorReportsEnvFileSourcesAndRedacts(t *testing.T) {
	const apiKey = "doctor-env-file-secret"
	server := fakeAuthPlane(t, apiKey)
	defer server.Close()

	workDir := t.TempDir()
	envPath := filepath.Join(workDir, ".env")
	writeTestEnvFile(t, envPath, discoveryEnv(server.URL, apiKey))

	res := runCLIInDir(t, workDir, nil, "doctor", "--for-agent", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, apiKey)
	env := parseEnvelope(t, res.stdout)
	checks := env["data"].(map[string]any)["checks"].([]any)
	apiKeyCheck := findDoctorCheck(t, checks, "api_key")
	gotPath, _ := apiKeyCheck["path"].(string)
	if apiKeyCheck["source"] != "env_file" || canonicalTestPath(t, gotPath) != canonicalTestPath(t, envPath) || apiKeyCheck["message"] != "configured" {
		t.Fatalf("unexpected api_key check: %#v", apiKeyCheck)
	}
}

func TestMissingExplicitEnvFileIsTyped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.env")
	res := runCLI(t, nil, "--env-file", missing, "auth", "status", "--format", "json")
	res.assertExit(t, 1)
	assertErrorCode(t, res.stdout, "ENV_FILE_NOT_FOUND")
}

func fakeAuthPlane(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/me/" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Fatalf("unexpected X-API-Key: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"user-1","email":"agent@example.test"}`)
	}))
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		return abs
	}
	return path
}

func findDoctorCheck(t *testing.T, checks []any, name string) map[string]any {
	t.Helper()
	for _, raw := range checks {
		check := raw.(map[string]any)
		if check["name"] == name {
			return check
		}
	}
	t.Fatalf("doctor check not found: %s", name)
	return nil
}
