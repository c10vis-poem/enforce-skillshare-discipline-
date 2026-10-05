package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
)

func TestCommitDryRunWarnsSourceLink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "skillshare", "config.yaml"))
	source := filepath.Join(root, "skills")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "init", "-q")
	if err := os.Symlink(t.TempDir(), filepath.Join(source, "_dev-skills")); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Source: source}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := cmdCommit([]string{"--dry-run"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "/_dev-skills") || !strings.Contains(output, "commit will stage source link") {
		t.Fatalf("missing warning: %s", output)
	}
	if got := runGit(t, source, "ls-files"); got != "" {
		t.Fatalf("dry run staged files: %s", got)
	}
}
