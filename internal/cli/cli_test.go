package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{"Usage:", "search", "download", "read", "sources", "version"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected help output to contain %q, got %q", want, output)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	var commandStdout bytes.Buffer
	var commandStderr bytes.Buffer
	commandExit := Run([]string{"version"}, &commandStdout, &commandStderr)
	if commandExit != 0 {
		t.Fatalf("expected version command exit 0, got %d", commandExit)
	}

	var flagStdout bytes.Buffer
	var flagStderr bytes.Buffer
	flagExit := Run([]string{"--version"}, &flagStdout, &flagStderr)
	if flagExit != 0 {
		t.Fatalf("expected --version exit 0, got %d", flagExit)
	}

	if commandStdout.String() != flagStdout.String() {
		t.Fatalf("expected matching version output, got command=%q flag=%q", commandStdout.String(), flagStdout.String())
	}

	if strings.TrimSpace(commandStdout.String()) == "" {
		t.Fatalf("expected non-empty version output")
	}

	if commandStderr.Len() != 0 || flagStderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got command=%q flag=%q", commandStderr.String(), flagStderr.String())
	}
}

func TestRootMisuse(t *testing.T) {
	t.Parallel()

	t.Run("no subcommand returns help", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run(nil, &stdout, &stderr)
		if exitCode == 0 {
			t.Fatalf("expected non-zero exit code")
		}

		if stdout.Len() != 0 {
			t.Fatalf("expected empty stdout, got %q", stdout.String())
		}

		if !strings.Contains(stderr.String(), "Usage:") {
			t.Fatalf("expected help text on stderr, got %q", stderr.String())
		}

		assertNoPanicText(t, stdout.String(), stderr.String())
	})

	t.Run("unknown command returns structured error", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run([]string{"nope"}, &stdout, &stderr)
		if exitCode == 0 {
			t.Fatalf("expected non-zero exit code")
		}

		assertJSONInvalidUsage(t, stdout.String(), "unknown command")
		assertNoPanicText(t, stdout.String(), stderr.String())
	})

	t.Run("unknown global flag returns structured error", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		exitCode := Run([]string{"--bogus"}, &stdout, &stderr)
		if exitCode == 0 {
			t.Fatalf("expected non-zero exit code")
		}

		assertJSONInvalidUsage(t, stdout.String(), "unknown flag")
		assertNoPanicText(t, stdout.String(), stderr.String())
	})
}

func TestWarningsStayOnStderr(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	cwd := filepath.Join(repoRoot, "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(cwd, ".env"), []byte("MALFORMED sentinel-secret\nPAPER_SEARCH_MCP_UNPAYWALL_EMAIL=ok@example.com\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		workingDir:     cwd,
		repositoryRoot: repoRoot,
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected stdout to remain valid json, got %q: %v", stdout.String(), err)
	}

	if payload.Status != "ok" {
		t.Fatalf("expected ok status, got %#v", payload)
	}

	if !strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("expected warning on stderr, got %q", stderr.String())
	}
}

func TestSecretsAreRedacted(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	cwd := filepath.Join(repoRoot, "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	secret := "sentinel-secret-value"
	if err := os.WriteFile(filepath.Join(cwd, ".env"), []byte("BROKEN "+secret+"\nPAPER_SEARCH_MCP_CORE_API_KEY="+secret+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithOptions([]string{"sources", "--format", "json"}, &stdout, &stderr, runOptions{
		environ:        []string{"PAPER_SEARCH_MCP_IEEE_API_KEY=" + secret},
		workingDir:     cwd,
		repositoryRoot: repoRoot,
	})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d with stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	for _, output := range []string{stdout.String(), stderr.String()} {
		if strings.Contains(output, secret) {
			t.Fatalf("expected secret to be redacted from output, got %q", output)
		}
	}
}

func assertJSONInvalidUsage(t *testing.T, payload string, wantMessage string) {
	t.Helper()

	var response struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("expected valid json error payload, got %q: %v", payload, err)
	}

	if response.Status != "error" {
		t.Fatalf("expected status=error, got %#v", response)
	}

	if response.Error.Code != "invalid_usage" {
		t.Fatalf("expected invalid_usage code, got %#v", response)
	}

	if !strings.Contains(response.Error.Message, wantMessage) {
		t.Fatalf("expected error message to contain %q, got %#v", wantMessage, response)
	}
}

func assertNoPanicText(t *testing.T, outputs ...string) {
	t.Helper()

	for _, output := range outputs {
		if strings.Contains(strings.ToLower(output), "panic") || strings.Contains(strings.ToLower(output), "traceback") {
			t.Fatalf("expected no panic or traceback text, got %q", output)
		}
	}
}
