package release

import "path/filepath"

const BinaryName = "search-paper-cli"

type Target struct {
	OS   string
	Arch string
}

var SupportedTargets = []Target{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

func (t Target) ArtifactDirName() string {
	return BinaryName + "_" + t.OS + "_" + t.Arch
}

func (t Target) ArchiveName() string {
	return t.ArtifactDirName() + t.archiveExtension()
}

func (t Target) BinaryFileName() string {
	if t.OS == "windows" {
		return BinaryName + ".exe"
	}
	return BinaryName
}

func (t Target) archiveExtension() string {
	if t.OS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

type Layout struct {
	Target      Target
	DistDir     string
	ArtifactDir string
	BinaryPath  string
	ArchivePath string
}

func ArtifactLayout(repoRoot string, target Target) Layout {
	distDir := filepath.Join(repoRoot, "dist")
	artifactDir := filepath.Join(distDir, target.ArtifactDirName())
	return Layout{
		Target:      target,
		DistDir:     distDir,
		ArtifactDir: artifactDir,
		BinaryPath:  filepath.Join(artifactDir, target.BinaryFileName()),
		ArchivePath: filepath.Join(distDir, target.ArchiveName()),
	}
}
