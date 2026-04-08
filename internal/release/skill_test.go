package release

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillRunScriptUsesSkillLocalEnvFileFromAnyWorkingDirectory(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	repoRoot := findRepoRoot(t)
	skillRoot := filepath.Join(t.TempDir(), "search-paper")
	if err := os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	copyFile(t, filepath.Join(repoRoot, "skills", "search-paper", "scripts", "ensure-search-paper-cli.sh"), filepath.Join(skillRoot, "scripts", "ensure-search-paper-cli.sh"))
	copyFile(t, filepath.Join(repoRoot, "skills", "search-paper", "scripts", "run-search-paper-cli.sh"), filepath.Join(skillRoot, "scripts", "run-search-paper-cli.sh"))
	writeFile(t, filepath.Join(skillRoot, ".env.example"), "SEARCH_PAPER_UNPAYWALL_EMAIL=you@example.com\n")
	writeFile(t, filepath.Join(skillRoot, ".env"), strings.Join([]string{
		"SEARCH_PAPER_UNPAYWALL_EMAIL=skill@example.com",
		"SEARCH_PAPER_IEEE_API_KEY=ieee-from-skill",
		"",
	}, "\n"))

	cmd := exec.Command("/bin/sh", filepath.Join(skillRoot, "scripts", "run-search-paper-cli.sh"), "sources")
	cmd.Dir = t.TempDir()
	cmd.Env = append(filteredEnv(), "SEARCH_PAPER_CLI_BIN="+binaryPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("skill run script failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var payload artifactSourcesPayload
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid sources json, got %q: %v", stdout.String(), err)
	}

	if payload.Status != "ok" {
		t.Fatalf("expected ok payload, got %#v", payload)
	}

	assertSourceEnabled(t, payload.Sources, "ieee", true)
	assertSourceEnabled(t, payload.Sources, "acm", false)
}

func TestSkillEnsureScriptInstallsCLIWhenMissing(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	skillRoot := filepath.Join(t.TempDir(), "search-paper")
	if err := os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	copyFile(t, filepath.Join(repoRoot, "skills", "search-paper", "scripts", "ensure-search-paper-cli.sh"), filepath.Join(skillRoot, "scripts", "ensure-search-paper-cli.sh"))

	fakeBinDir := t.TempDir()
	fakeGoPath := filepath.Join(fakeBinDir, "go")
	writeFile(t, fakeGoPath, strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"if [ \"${1-}\" != \"install\" ]; then",
		"  echo \"unexpected go arguments: $*\" >&2",
		"  exit 1",
		"fi",
		"cat > \"$GOBIN/search-paper-cli\" <<'EOF'",
		"#!/bin/sh",
		"printf 'fake-search-paper-cli\\n'",
		"EOF",
		"chmod +x \"$GOBIN/search-paper-cli\"",
		"",
	}, "\n"))
	if err := os.Chmod(fakeGoPath, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", fakeGoPath, err)
	}

	cmd := exec.Command("/bin/sh", filepath.Join(skillRoot, "scripts", "ensure-search-paper-cli.sh"))
	cmd.Dir = skillRoot
	cmd.Env = []string{
		"PATH=" + fakeBinDir + ":/usr/bin:/bin",
		"HOME=" + t.TempDir(),
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ensure script failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	resolvedPath := strings.TrimSpace(stdout.String())
	wantPath := filepath.Join(skillRoot, "bin", "search-paper-cli")
	if resolvedPath != wantPath {
		t.Fatalf("expected installed binary path %q, got %q", wantPath, resolvedPath)
	}
	if !fileExists(wantPath) {
		t.Fatalf("expected installed binary at %q", wantPath)
	}
}
