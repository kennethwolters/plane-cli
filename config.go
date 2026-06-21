package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type fileConfig struct {
	BaseURL       string `json:"base_url,omitempty"`
	WorkspaceSlug string `json:"workspace_slug,omitempty"`
}

type effectiveConfig struct {
	APIKey        configValue `json:"api_key"`
	BaseURL       configValue `json:"base_url"`
	WorkspaceSlug configValue `json:"workspace_slug"`
	ConfigPath    string      `json:"config_path"`
	ConfigExists  bool        `json:"config_exists"`
}

type configValue struct {
	Value   string `json:"value,omitempty"`
	Secret  string `json:"-"`
	Source  string `json:"source"`
	Path    string `json:"path,omitempty"`
	Present bool   `json:"present"`
}

type configContext struct {
	EnvFileValues      map[string]string
	EnvFilePaths       map[string]string
	EnvFileDiagnostics []envFileDiagnostic
}

type envFileDiagnostic struct {
	Code    string
	Message string
	Fix     string
	Path    string
	OK      bool
}

func buildConfigContext(explicitEnvFile string) configContext {
	ctx := configContext{
		EnvFileValues: map[string]string{},
		EnvFilePaths:  map[string]string{},
	}

	if explicitEnvFile == "" {
		explicitEnvFile = os.Getenv("PLANE_CLI_ENV_FILE")
	}
	if explicitEnvFile != "" {
		ctx.loadEnvFile(explicitEnvFile, true)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ctx
	}
	cwdDotenv := filepath.Join(cwd, ".env")
	if fileExists(cwdDotenv) {
		ctx.loadEnvFile(cwdDotenv, false)
	}
	if ancestor := findAncestorDotenv(cwd); ancestor != "" {
		ctx.loadEnvFile(ancestor, false)
	}
	return ctx
}

func (ctx *configContext) loadEnvFile(path string, explicit bool) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	values, err := parseEnvFile(absPath)
	if err != nil {
		if explicit {
			code := "ENV_FILE_READ_ERROR"
			msg := "Could not read explicit env file: " + absPath
			if os.IsNotExist(err) {
				code = "ENV_FILE_NOT_FOUND"
				msg = "Explicit env file does not exist: " + absPath
			}
			ctx.EnvFileDiagnostics = append(ctx.EnvFileDiagnostics, envFileDiagnostic{
				Code:    code,
				Message: msg,
				Fix:     "Check the --env-file path or PLANE_CLI_ENV_FILE value.",
				Path:    absPath,
				OK:      false,
			})
		}
		return
	}
	if explicit {
		ctx.EnvFileDiagnostics = append(ctx.EnvFileDiagnostics, envFileDiagnostic{
			Code:    "ENV_FILE_LOADED",
			Message: "explicit env file loaded",
			Path:    absPath,
			OK:      true,
		})
	}
	for key, value := range values {
		if _, exists := ctx.EnvFileValues[key]; exists {
			continue
		}
		ctx.EnvFileValues[key] = value
		ctx.EnvFilePaths[key] = absPath
	}
}

func (ctx configContext) blockingEnvFileError() *cliError {
	for _, diag := range ctx.EnvFileDiagnostics {
		if !diag.OK {
			return newError(diag.Code, diag.Message, diag.Fix, true)
		}
	}
	return nil
}

func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		if key == "" {
			continue
		}
		values[key] = value
	}
	return values, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func findAncestorDotenv(cwd string) string {
	current := filepath.Dir(cwd)
	for current != "." && current != string(filepath.Separator) {
		candidate := filepath.Join(current, ".env")
		if fileExists(candidate) {
			return candidate
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	return ""
}

func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "plane-cli", "config.json")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".config", "plane-cli", "config.json")
	}
	return filepath.Join(".config", "plane-cli", "config.json")
}

func readFileConfig() (fileConfig, bool, string, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fileConfig{}, false, path, nil
	}
	if err != nil {
		return fileConfig{}, false, path, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, true, path, err
	}
	return cfg, true, path, nil
}

func writeFileConfig(cfg fileConfig) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func loadEffectiveConfig(configCtx configContext) (effectiveConfig, *cliError) {
	cfg, exists, path, err := readFileConfig()
	if err != nil {
		return effectiveConfig{}, newError("CONFIG_READ_ERROR", "Could not read config file.", "Check permissions or fix JSON in "+path+".", true)
	}
	eff := effectiveConfig{ConfigPath: path, ConfigExists: exists}
	eff.APIKey = configSourceValue("PLANE_API_KEY", configCtx, true)
	eff.BaseURL = configSourceValue("PLANE_BASE_URL", configCtx, false)
	if !eff.BaseURL.Present && cfg.BaseURL != "" {
		eff.BaseURL = configValue{Value: cfg.BaseURL, Source: "config", Path: path, Present: true}
	}
	eff.WorkspaceSlug = configSourceValue("PLANE_WORKSPACE_SLUG", configCtx, false)
	if !eff.WorkspaceSlug.Present && cfg.WorkspaceSlug != "" {
		eff.WorkspaceSlug = configValue{Value: cfg.WorkspaceSlug, Source: "config", Path: path, Present: true}
	}
	return eff, nil
}

func configSourceValue(key string, configCtx configContext, secret bool) configValue {
	if value, ok := getenv(key); ok {
		return valuedConfigValue(value, "env", "", secret)
	}
	if value, ok := configCtx.EnvFileValues[key]; ok && value != "" {
		return valuedConfigValue(value, "env_file", configCtx.EnvFilePaths[key], secret)
	}
	return configValue{Source: "missing", Present: false}
}

func valuedConfigValue(value, source, path string, secret bool) configValue {
	if secret {
		return configValue{Secret: value, Source: source, Path: path, Present: true}
	}
	return configValue{Value: value, Source: source, Path: path, Present: true}
}

func (a app) cmdConfig(args []string, configCtx configContext) int {
	if len(args) == 0 {
		return a.usageError("config requires a subcommand", "text")
	}
	sub := args[0]
	format, rest, err := parseFormat(args[1:])
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	switch sub {
	case "get":
		if len(rest) != 0 {
			return a.usageError("config get takes no positional arguments", format)
		}
		return a.cmdConfigGet(format, configCtx)
	case "set":
		return a.cmdConfigSet(format, rest)
	default:
		return a.usageError("unknown config subcommand: "+sub, format)
	}
}

func (a app) cmdConfigGet(format string, configCtx configContext) int {
	if envErr := configCtx.blockingEnvFileError(); envErr != nil {
		return a.writeCLIError(envErr, format)
	}
	eff, err := loadEffectiveConfig(configCtx)
	if err != nil {
		return a.writeCLIError(err, format)
	}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.config.v1", eff))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "config_path: %s\n", eff.ConfigPath)
	fmt.Fprintf(a.stdout, "base_url: %s (%s)\n", displayMissing(eff.BaseURL.Value), eff.BaseURL.Source)
	fmt.Fprintf(a.stdout, "workspace_slug: %s (%s)\n", displayMissing(eff.WorkspaceSlug.Value), eff.WorkspaceSlug.Source)
	fmt.Fprintf(a.stdout, "api_key: %s (%s)\n", presentText(eff.APIKey.Present), eff.APIKey.Source)
	return exitOK
}

func (a app) cmdConfigSet(format string, args []string) int {
	if len(args) != 2 {
		return a.usageError("config set requires <key> <value>", format)
	}
	key, value := args[0], args[1]
	if isSecretKey(key) {
		return a.writeCLIError(newError("CONFIG_WRITE_REJECTED_SECRET", "Refusing to store secret config key: "+key, "Provide the API key via PLANE_API_KEY instead.", false, "PLANE_API_KEY=... plane-cli auth status"), format)
	}
	cfg, _, _, readErr := readFileConfig()
	if readErr != nil {
		return a.writeCLIError(newError("CONFIG_READ_ERROR", "Could not read config file.", "Check config file permissions or JSON syntax.", true), format)
	}
	switch key {
	case "base_url":
		cfg.BaseURL = value
	case "workspace_slug":
		cfg.WorkspaceSlug = value
	default:
		return a.usageError("unsupported config key: "+key, format)
	}
	if err := writeFileConfig(cfg); err != nil {
		return a.writeCLIError(newError("CONFIG_WRITE_ERROR", "Could not write config file.", "Check permissions for "+configPath()+".", true), format)
	}
	data := map[string]string{"key": key, "config_path": configPath()}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.config.set.v1", data))
		return exitOK
	}
	fmt.Fprintf(a.stdout, "set %s in %s\n", key, configPath())
	return exitOK
}

func isSecretKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "api_key") || strings.Contains(k, "apikey") || strings.Contains(k, "pat") || strings.Contains(k, "token") || strings.Contains(k, "secret")
}

func displayMissing(value string) string {
	if value == "" {
		return "<missing>"
	}
	return value
}

func presentText(present bool) string {
	if present {
		return "present"
	}
	return "missing"
}
