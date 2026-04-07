package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const (
	exitCodeOK           = 0
	exitCodeInvalidUsage = 2
)

const defaultVersion = "search-paper-cli dev"

var version = defaultVersion

type errorResponse struct {
	Status string `json:"status"`
	Error  struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

type command struct {
	name        string
	summary     string
	description string
	run         func(args []string, stdout, stderr io.Writer) int
}

var commands = []command{
	{
		name:        "sources",
		summary:     "List configured paper sources.",
		description: "Inspect the source registry and source capabilities.",
		run:         runPlaceholderCommand("sources", "Inspect the source registry and source capabilities."),
	},
	{
		name:        "search",
		summary:     "Search for papers across one or more sources.",
		description: "Search registered sources and return normalized paper results.",
		run:         runPlaceholderCommand("search", "Search registered sources and return normalized paper results."),
	},
	{
		name:        "download",
		summary:     "Download paper full text when supported.",
		description: "Download source-native or fallback paper full text into a target directory.",
		run:         runPlaceholderCommand("download", "Download source-native or fallback paper full text into a target directory."),
	},
	{
		name:        "read",
		summary:     "Read paper content when supported.",
		description: "Fetch and extract paper content from a source-native or fallback retrieval path.",
		run:         runPlaceholderCommand("read", "Fetch and extract paper content from a source-native or fallback retrieval path."),
	},
	{
		name:        "version",
		summary:     "Print the CLI version.",
		description: "Print the concise CLI version string.",
		run: func(args []string, stdout, stderr io.Writer) int {
			if len(args) != 0 {
				return writeInvalidUsage(stdout, "unknown arguments for version command", map[string]any{
					"command": "version",
					"args":    args,
				})
			}

			_, _ = fmt.Fprintln(stdout, version)
			return exitCodeOK
		},
	},
}

func Run(args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			_, _ = io.WriteString(stdout, rootHelp())
			return exitCodeOK
		}
	}

	global := flag.NewFlagSet("search-paper-cli", flag.ContinueOnError)
	global.SetOutput(io.Discard)

	showVersion := global.Bool("version", false, "Print version information and exit.")
	global.BoolVar(showVersion, "v", false, "Print version information and exit.")

	if err := global.Parse(args); err != nil {
		return writeInvalidUsage(stdout, normalizeFlagError(err), nil)
	}

	if *showVersion {
		_, _ = fmt.Fprintln(stdout, version)
		return exitCodeOK
	}

	remaining := global.Args()
	if len(remaining) == 0 {
		_, _ = io.WriteString(stderr, rootHelp())
		return exitCodeInvalidUsage
	}

	name := remaining[0]
	if name == "help" {
		return runHelp(remaining[1:], stdout)
	}

	cmd, ok := lookupCommand(name)
	if !ok {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown command %q", name), map[string]any{
			"valid_commands": commandNames(),
		})
	}

	return cmd.run(remaining[1:], stdout, stderr)
}

func runHelp(args []string, stdout io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stdout, rootHelp())
		return exitCodeOK
	}

	cmd, ok := lookupCommand(args[0])
	if !ok {
		return writeInvalidUsage(stdout, fmt.Sprintf("unknown command %q", args[0]), map[string]any{
			"valid_commands": commandNames(),
		})
	}

	_, _ = io.WriteString(stdout, commandHelp(cmd))
	return exitCodeOK
}

func lookupCommand(name string) (command, bool) {
	for _, cmd := range commands {
		if cmd.name == name {
			return cmd, true
		}
	}
	return command{}, false
}

func commandNames() []string {
	names := make([]string, 0, len(commands))
	for _, cmd := range commands {
		names = append(names, cmd.name)
	}
	return names
}

func rootHelp() string {
	var b strings.Builder
	b.WriteString("Usage:\n")
	b.WriteString("  search-paper-cli <command> [flags]\n")
	b.WriteString("  search-paper-cli --version\n\n")
	b.WriteString("Commands:\n")
	for _, cmd := range commands {
		b.WriteString(fmt.Sprintf("  %-10s %s\n", cmd.name, cmd.summary))
	}
	b.WriteString("\nGlobal Flags:\n")
	b.WriteString("  --help      Show help for the root command.\n")
	b.WriteString("  --version   Print version information and exit.\n")
	return b.String()
}

func commandHelp(cmd command) string {
	var b strings.Builder
	b.WriteString("Usage:\n")
	b.WriteString(fmt.Sprintf("  search-paper-cli %s\n\n", cmd.name))
	b.WriteString(fmt.Sprintf("%s\n", cmd.description))
	return b.String()
}

func runPlaceholderCommand(name, description string) func(args []string, stdout, stderr io.Writer) int {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) > 0 {
			switch args[0] {
			case "--help", "-h":
				_, _ = io.WriteString(stdout, placeholderCommandHelp(name, description))
				return exitCodeOK
			default:
				return writeInvalidUsage(stdout, fmt.Sprintf("unknown argument %q for %s command", args[0], name), map[string]any{
					"command": name,
					"args":    args,
				})
			}
		}

		_, _ = io.WriteString(stderr, placeholderCommandHelp(name, description))
		return exitCodeInvalidUsage
	}
}

func placeholderCommandHelp(name, description string) string {
	return commandHelp(command{
		name:        name,
		description: description,
	})
}

func normalizeFlagError(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, flag.ErrHelp):
		return "help requested"
	case strings.Contains(err.Error(), "flag provided but not defined"):
		name := strings.TrimSpace(strings.TrimPrefix(err.Error(), "flag provided but not defined:"))
		return fmt.Sprintf("unknown flag %q", name)
	default:
		return err.Error()
	}
}

func writeInvalidUsage(stdout io.Writer, message string, details map[string]any) int {
	response := errorResponse{Status: "error"}
	response.Error.Code = "invalid_usage"
	response.Error.Message = message
	response.Error.Details = details

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(response)
	return exitCodeInvalidUsage
}
