package release

import "path/filepath"

const (
	BinaryName      = "search-paper-cli"
	TargetOS        = "linux"
	TargetArch      = "amd64"
	ArtifactDirName = BinaryName + "_" + TargetOS + "_" + TargetArch
	ArchiveName     = ArtifactDirName + ".tar.gz"
)

type Layout struct {
	DistDir     string
	ArtifactDir string
	BinaryPath  string
	ArchivePath string
}

func ArtifactLayout(repoRoot string) Layout {
	distDir := filepath.Join(repoRoot, "dist")
	artifactDir := filepath.Join(distDir, ArtifactDirName)
	return Layout{
		DistDir:     distDir,
		ArtifactDir: artifactDir,
		BinaryPath:  filepath.Join(artifactDir, BinaryName),
		ArchivePath: filepath.Join(distDir, ArchiveName),
	}
}
