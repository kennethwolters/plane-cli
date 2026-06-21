package main

import (
	"context"
	"fmt"
	"runtime"
)

type doctorData struct {
	ForAgent bool          `json:"for_agent"`
	Healthy  bool          `json:"healthy"`
	Checks   []doctorCheck `json:"checks"`
}

type doctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
	Source  string `json:"source,omitempty"`
	Path    string `json:"path,omitempty"`
	Code    string `json:"code,omitempty"`
}

func (a app) cmdDoctor(ctx context.Context, args []string, configCtx configContext) int {
	args, forAgent := hasFlag(args, "--for-agent")
	format, rest, err := parseFormat(args)
	if err != nil {
		return a.writeCLIError(err, "json")
	}
	if len(rest) != 0 {
		return a.usageError("doctor takes no positional arguments", format)
	}

	eff, cfgErr := loadEffectiveConfig(configCtx)
	checks := []doctorCheck{
		{Name: "binary", OK: true, Message: "plane-cli " + version},
		{Name: "platform", OK: runtime.GOOS == "linux" || runtime.GOOS == "darwin", Message: runtime.GOOS + "/" + runtime.GOARCH, Fix: platformFix()},
	}
	checks = append(checks, doctorEnvFileChecks(configCtx)...)
	if cfgErr != nil {
		checks = append(checks, doctorCheck{Name: "config_file", OK: false, Message: cfgErr.Message, Fix: cfgErr.Fix})
	} else {
		checks = append(checks,
			doctorConfigFile(eff.ConfigExists, eff.ConfigPath),
			doctorConfigValue("base_url", eff.BaseURL, "Set PLANE_BASE_URL or run: plane-cli config set base_url <url>"),
			doctorConfigValue("workspace_slug", eff.WorkspaceSlug, "Set PLANE_WORKSPACE_SLUG or run: plane-cli config set workspace_slug <slug>"),
			doctorConfigValue("api_key", eff.APIKey, "Set PLANE_API_KEY in the environment or an env file."),
		)
		if validateRequiredConfig(eff) == nil {
			client := newPlaneClient(eff, a.client)
			if _, err := client.getMe(ctx); err != nil {
				checks = append(checks, doctorCheck{Name: "plane_api", OK: false, Message: err.Message, Fix: err.Fix})
			} else {
				checks = append(checks, doctorCheck{Name: "plane_api", OK: true, Message: "/api/v1/users/me/ succeeded"})
			}
		} else {
			checks = append(checks, doctorCheck{Name: "plane_api", OK: false, Message: "skipped because required config is missing", Fix: "Fix missing config checks first."})
		}
	}
	data := doctorData{ForAgent: forAgent, Healthy: allChecksOK(checks), Checks: checks}
	if format == "json" {
		writeJSON(a.stdout, okEnvelope("plane.doctor.v1", data))
	} else {
		for _, check := range checks {
			status := "ok"
			if !check.OK {
				status = "fail"
			}
			fmt.Fprintf(a.stdout, "%s: %s - %s\n", check.Name, status, check.Message)
		}
	}
	if data.Healthy {
		return exitOK
	}
	return exitError
}

func doctorEnvFileChecks(configCtx configContext) []doctorCheck {
	checks := make([]doctorCheck, 0, len(configCtx.EnvFileDiagnostics))
	for _, diag := range configCtx.EnvFileDiagnostics {
		checks = append(checks, doctorCheck{Name: "env_file", OK: diag.OK, Message: diag.Message, Fix: diag.Fix, Path: diag.Path, Code: diag.Code})
	}
	return checks
}

func doctorConfigFile(exists bool, path string) doctorCheck {
	if exists {
		return doctorCheck{Name: "config_file", OK: true, Message: path}
	}
	return doctorCheck{Name: "config_file", OK: true, Message: "not found at " + path + "; env-only configuration is allowed"}
}

func doctorConfigValue(name string, value configValue, fix string) doctorCheck {
	if value.Present {
		msg := "configured"
		if name != "api_key" {
			msg = value.Value
		}
		return doctorCheck{Name: name, OK: true, Message: msg, Source: value.Source, Path: value.Path}
	}
	return doctorCheck{Name: name, OK: false, Message: "missing", Fix: fix, Source: value.Source, Path: value.Path}
}

func allChecksOK(checks []doctorCheck) bool {
	for _, check := range checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func platformFix() string {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		return ""
	}
	return "V1 supports Linux and macOS."
}
