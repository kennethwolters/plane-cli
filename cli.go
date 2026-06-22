package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

type app struct {
	stdout io.Writer
	stderr io.Writer
	client *http.Client
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	a := app{stdout: stdout, stderr: stderr, client: http.DefaultClient}
	global, rest, err := parseGlobalFlags(args)
	if err != nil {
		return a.writeCLIError(err, "text")
	}
	configCtx := buildConfigContext(global.EnvFile)
	return a.run(ctx, rest, configCtx)
}

type globalFlags struct {
	EnvFile string
}

func parseGlobalFlags(args []string) (globalFlags, []string, *cliError) {
	var flags globalFlags
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--env-file":
			if i+1 >= len(args) {
				return flags, out, newError("USAGE_ERROR", "--env-file requires a path.", "Pass --env-file <path> before the command.", false)
			}
			flags.EnvFile = args[i+1]
			i++
		case strings.HasPrefix(arg, "--env-file="):
			flags.EnvFile = strings.TrimPrefix(arg, "--env-file=")
		default:
			out = append(out, args[i:]...)
			return flags, out, nil
		}
	}
	return flags, out, nil
}

func (a app) run(ctx context.Context, args []string, configCtx configContext) int {
	if len(args) == 0 {
		printUsage(a.stdout)
		return exitOK
	}
	if args[0] == "--version" || args[0] == "-version" {
		fmt.Fprintln(a.stdout, version)
		return exitOK
	}

	switch args[0] {
	case "version":
		format, rest, err := parseFormat(args[1:])
		if err != nil {
			return a.writeCLIError(err, "json")
		}
		if len(rest) != 0 {
			return a.usageError("version takes no positional arguments", format)
		}
		return a.cmdVersion(format)
	case "config":
		return a.cmdConfig(args[1:], configCtx)
	case "auth":
		return a.cmdAuth(ctx, args[1:], configCtx)
	case "doctor":
		return a.cmdDoctor(ctx, args[1:], configCtx)
	case "resolve":
		return a.cmdResolve(ctx, args[1:], configCtx)
	case "project":
		return a.cmdProject(ctx, args[1:], configCtx)
	case "state":
		return a.cmdState(ctx, args[1:], configCtx)
	case "member":
		return a.cmdMember(ctx, args[1:], configCtx)
	case "work":
		return a.cmdWork(ctx, args[1:], configCtx)
	case "search":
		return a.cmdSearch(ctx, args[1:], configCtx)
	case "help", "--help", "-h":
		printUsage(a.stdout)
		return exitOK
	default:
		return a.usageError("unknown command: "+args[0], "text")
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "plane-cli is a Plane.so CLI.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  plane-cli [--env-file <path>] <command>")
	fmt.Fprintln(w, "  plane-cli --version")
	fmt.Fprintln(w, "  plane-cli version [--format text|json]")
	fmt.Fprintln(w, "  plane-cli config get [--format text|json]")
	fmt.Fprintln(w, "  plane-cli config set <base_url|workspace_slug> <value> [--format text|json]")
	fmt.Fprintln(w, "  plane-cli auth status [--format text|json]")
	fmt.Fprintln(w, "  plane-cli doctor [--for-agent] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli resolve <PROJECT-123> [--format text|json] [--no-cache]")
	fmt.Fprintln(w, "  plane-cli project list [--format text|json]")
	fmt.Fprintln(w, "  plane-cli project get <project> [--format text|json]")
	fmt.Fprintln(w, "  plane-cli state list --project <project> [--format text|json]")
	fmt.Fprintln(w, "  plane-cli member list --project <project> [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work list --project <project> [--state-group <group>] [--limit <n>] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work get <PROJECT-123> [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work create --project <project> --title <title> [--description-html <html>|--description-file <path>] [--priority <priority>] [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work edit <PROJECT-123> [--title <title>] [--description-html <html>|--description-file <path>] [--priority <priority>] [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work comment <PROJECT-123> (--html <html>|--html-file <path>) [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work comments <PROJECT-123> [--limit <n>] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work start <PROJECT-123> [--reason <text>] [--evidence <text>] [--pr <url-or-number>] [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work complete <PROJECT-123> --evidence <text> [--reason <text>] [--pr <url-or-number>] [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work reopen <PROJECT-123> --reason <text> [--evidence <text>] [--pr <url-or-number>] [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli work cancel <PROJECT-123> --reason <text> [--evidence <text>] [--pr <url-or-number>] [--dry-run|--apply] [--verify] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli search <query> [--project <project>] [--max-results <n>] [--format text|json]")
}

func parseFormat(args []string) (string, []string, *cliError) {
	format := "text"
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--format":
			if i+1 >= len(args) {
				return format, out, newError("UNSUPPORTED_FORMAT", "--format requires a value.", "Use --format text or --format json.", false, "plane-cli version --format json")
			}
			format = args[i+1]
			i++
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		default:
			out = append(out, arg)
		}
	}
	if format != "text" && format != "json" {
		return format, out, newError("UNSUPPORTED_FORMAT", "Unsupported output format: "+format, "Use --format text or --format json.", false, "plane-cli version --format json")
	}
	return format, out, nil
}

func hasFlag(args []string, name string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == name {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return out, found
}

func parseRequiredStringFlag(args []string, name, missingMessage string) (string, []string, *cliError) {
	value, rest, ok := parseStringFlag(args, name)
	if !ok || value == "" {
		return "", rest, newError("MISSING_PROJECT_REFERENCE", missingMessage, "Pass "+name+" with a Plane project identifier or UUID.", false)
	}
	return value, rest, nil
}

func parseStringFlag(args []string, name string) (string, []string, bool) {
	out := make([]string, 0, len(args))
	found := false
	value := ""
	prefix := name + "="
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == name:
			found = true
			if i+1 < len(args) {
				value = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, prefix):
			found = true
			value = strings.TrimPrefix(arg, prefix)
		default:
			out = append(out, arg)
		}
	}
	return value, out, found
}

func (a app) usageError(message, format string) int {
	return a.writeCLIError(newError("USAGE_ERROR", message, "Run plane-cli help for usage.", false, "plane-cli help"), format)
}

func (a app) writeCLIError(err *cliError, format string) int {
	if format == "json" {
		writeJSON(a.stdout, errorEnvelope(err))
	} else {
		fmt.Fprintf(a.stderr, "%s: %s\n", err.Code, err.Message)
		if err.Fix != "" {
			fmt.Fprintf(a.stderr, "fix: %s\n", err.Fix)
		}
	}
	if err.Code == "USAGE_ERROR" || err.Code == "UNSUPPORTED_FORMAT" {
		return exitUsage
	}
	return exitError
}

func getenv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}
