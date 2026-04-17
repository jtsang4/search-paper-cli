package release

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstalledSkillContextUsesDirectCLIWithGlobalConfig(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	repoRoot := findRepoRoot(t)
	skillRoot := filepath.Join(t.TempDir(), "search-paper")
	if err := os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	copyFile(t, filepath.Join(repoRoot, "skills", "search-paper", "scripts", "ensure-search-paper-cli.sh"), filepath.Join(skillRoot, "scripts", "ensure-search-paper-cli.sh"))
	copyFile(t, binaryPath, filepath.Join(skillRoot, "bin", "search-paper-cli"))
	writeFile(t, filepath.Join(skillRoot, ".env"), "SEARCH_PAPER_ACM_API_KEY=acm-from-skill\n")
	explicitEnvFile := filepath.Join(t.TempDir(), "legacy.env")
	writeFile(t, explicitEnvFile, "SEARCH_PAPER_ACM_API_KEY=acm-from-explicit\n")

	homeDir := t.TempDir()
	writeGlobalConfig(t, homeDir, "config.yaml", "ieee_api_key: ieee-from-global\n")

	cmd := exec.Command("/bin/sh", "-c", "search-paper-cli sources")
	cmd.Dir = skillRoot
	cmd.Env = []string{
		"HOME=" + homeDir,
		"PATH=" + filepath.Join(skillRoot, "bin") + ":/usr/bin:/bin",
		"SEARCH_PAPER_ENV_FILE=" + explicitEnvFile,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("direct skill-context command failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
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

func TestSkillEnsureScriptResolvesPlatformLocalBinary(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	skillRoot := filepath.Join(t.TempDir(), "search-paper")
	if err := os.MkdirAll(filepath.Join(skillRoot, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	copyFile(t, filepath.Join(repoRoot, "skills", "search-paper", "scripts", "ensure-search-paper-cli.sh"), filepath.Join(skillRoot, "scripts", "ensure-search-paper-cli.sh"))

	localBinary := filepath.Join(skillRoot, "bin", "search-paper-cli")
	if runtime.GOOS == "windows" {
		localBinary += ".exe"
	}
	writeFile(t, localBinary, "#!/bin/sh\nprintf 'local-binary\\n'\n")
	if err := os.Chmod(localBinary, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", localBinary, err)
	}

	cmd := exec.Command("/bin/sh", filepath.Join(skillRoot, "scripts", "ensure-search-paper-cli.sh"))
	cmd.Dir = skillRoot
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=/usr/bin:/bin",
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("ensure script failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	resolvedPath := strings.TrimSpace(stdout.String())
	if resolvedPath != localBinary {
		t.Fatalf("expected local binary path %q, got %q", localBinary, resolvedPath)
	}
}

func TestInstalledSkillContextDirectCommandsNeedNoWrapperPreflight(t *testing.T) {
	t.Parallel()

	binaryPath := buildArtifactBinary(t)
	skillRoot := filepath.Join(t.TempDir(), "search-paper")
	if err := os.MkdirAll(filepath.Join(skillRoot, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	copyFile(t, binaryPath, filepath.Join(skillRoot, "bin", "search-paper-cli"))

	versionCmd := exec.Command("/bin/sh", "-c", "search-paper-cli version")
	versionCmd.Dir = skillRoot
	versionCmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + filepath.Join(skillRoot, "bin") + ":/usr/bin:/bin",
	}
	versionOutput, err := versionCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version command failed: %v\noutput=%s", err, string(versionOutput))
	}
	if !strings.Contains(string(versionOutput), "search-paper-cli") {
		t.Fatalf("expected version output, got %q", string(versionOutput))
	}

	sourcesCmd := exec.Command("/bin/sh", "-c", "search-paper-cli sources")
	sourcesCmd.Dir = skillRoot
	sourcesCmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + filepath.Join(skillRoot, "bin") + ":/usr/bin:/bin",
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	sourcesCmd.Stdout = &stdout
	sourcesCmd.Stderr = &stderr
	if err := sourcesCmd.Run(); err != nil {
		t.Fatalf("sources command failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	var payload artifactSourcesPayload
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid sources json, got %q: %v", stdout.String(), err)
	}
	if payload.Status != "ok" {
		t.Fatalf("expected ok payload, got %#v", payload)
	}

	assertSourceEnabled(t, payload.Sources, "ieee", false)
	assertSourceEnabled(t, payload.Sources, "acm", false)
}
