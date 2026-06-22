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

func TestEnvFileFlagPrecedenceBeatsCwdDotenv(t *testing.T) {
	workDir := t.TempDir()
	explicitPath := filepath.Join(workDir, "explicit.env")
	cwdPath := filepath.Join(workDir, ".env")
	writeTestEnvFile(t, explicitPath, discoveryEnv("https://explicit.example.test", "explicit-env-file-secret"))
	writeTestEnvFile(t, cwdPath, discoveryEnv("https://cwd.example.test", "cwd-env-file-secret"))

	res := runCLIInDir(t, workDir, nil, "--env-file", explicitPath, "config", "get", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, "explicit-env-file-secret")
	res.assertNoSecret(t, "cwd-env-file-secret")
	assertConfigGetEnvFileSource(t, res.stdout, explicitPath, "https://explicit.example.test", "development")
}

func TestEnvFileEnvVarPrecedenceBeatsCwdDotenv(t *testing.T) {
	workDir := t.TempDir()
	envPath := filepath.Join(workDir, "from-env.env")
	cwdPath := filepath.Join(workDir, ".env")
	writeTestEnvFile(t, envPath, discoveryEnv("https://env-var.example.test", "env-var-env-file-secret"))
	writeTestEnvFile(t, cwdPath, discoveryEnv("https://cwd.example.test", "cwd-env-file-secret"))

	res := runCLIInDir(t, workDir, map[string]string{"PLANE_CLI_ENV_FILE": envPath}, "config", "get", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, "env-var-env-file-secret")
	res.assertNoSecret(t, "cwd-env-file-secret")
	assertConfigGetEnvFileSource(t, res.stdout, envPath, "https://env-var.example.test", "development")
}

func TestCwdDotenvPrecedenceBeatsAncestorDotenv(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "packages", "cli")
	cwdPath := filepath.Join(nested, ".env")
	ancestorPath := filepath.Join(root, ".env")
	writeTestEnvFile(t, ancestorPath, discoveryEnv("https://ancestor.example.test", "ancestor-env-file-secret"))
	writeTestEnvFile(t, cwdPath, discoveryEnv("https://cwd.example.test", "cwd-env-file-secret"))

	res := runCLIInDir(t, nested, nil, "config", "get", "--format", "json")
	res.assertExit(t, 0)
	res.assertNoSecret(t, "cwd-env-file-secret")
	res.assertNoSecret(t, "ancestor-env-file-secret")
	assertConfigGetEnvFileSource(t, res.stdout, cwdPath, "https://cwd.example.test", "development")
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

func TestMissingExplicitEnvFileDiagnosticsAreTyped(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.env")
	const secret = "missing-env-file-diagnostic-secret"

	t.Run("config get", func(t *testing.T) {
		res := runCLI(t, map[string]string{"PLANE_API_KEY": secret}, "--env-file", missing, "config", "get", "--format", "json")
		res.assertExit(t, 1)
		res.assertNoSecret(t, secret)
		assertErrorCode(t, res.stdout, "ENV_FILE_NOT_FOUND")
	})

	t.Run("auth status", func(t *testing.T) {
		res := runCLI(t, map[string]string{"PLANE_API_KEY": secret}, "--env-file", missing, "auth", "status", "--format", "json")
		res.assertExit(t, 1)
		res.assertNoSecret(t, secret)
		assertErrorCode(t, res.stdout, "ENV_FILE_NOT_FOUND")
	})

	t.Run("doctor", func(t *testing.T) {
		res := runCLI(t, map[string]string{"PLANE_API_KEY": secret}, "--env-file", missing, "doctor", "--for-agent", "--format", "json")
		res.assertExit(t, 1)
		res.assertNoSecret(t, secret)
		env := parseEnvelope(t, res.stdout)
		checks := env["data"].(map[string]any)["checks"].([]any)
		envFileCheck := findDoctorCheck(t, checks, "env_file")
		if envFileCheck["ok"] != false || envFileCheck["code"] != "ENV_FILE_NOT_FOUND" {
			t.Fatalf("unexpected env_file check: %#v", envFileCheck)
		}
		gotPath, _ := envFileCheck["path"].(string)
		if canonicalTestPath(t, gotPath) != canonicalTestPath(t, missing) {
			t.Fatalf("env_file check path %q, want %q", gotPath, missing)
		}
	})
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

func assertConfigGetEnvFileSource(t *testing.T, stdout, wantPath, wantBaseURL, wantWorkspace string) {
	t.Helper()
	env := parseEnvelope(t, stdout)
	if env["schema"] != "plane.config.v1" {
		t.Fatalf("unexpected schema: %#v", env)
	}
	data := env["data"].(map[string]any)
	for name, wantValue := range map[string]string{
		"base_url":       wantBaseURL,
		"workspace_slug": wantWorkspace,
	} {
		value := data[name].(map[string]any)
		gotPath, _ := value["path"].(string)
		if value["source"] != "env_file" || canonicalTestPath(t, gotPath) != canonicalTestPath(t, wantPath) || value["value"] != wantValue || value["present"] != true {
			t.Fatalf("unexpected %s config value: %#v", name, value)
		}
	}
	apiKey := data["api_key"].(map[string]any)
	gotPath, _ := apiKey["path"].(string)
	if apiKey["source"] != "env_file" || canonicalTestPath(t, gotPath) != canonicalTestPath(t, wantPath) || apiKey["value"] != nil || apiKey["present"] != true {
		t.Fatalf("unexpected api_key config value: %#v", apiKey)
	}
}
