package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/testutil"
)

func TestUpdateFollowedCheckoutResolution(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "skills")
	t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "config.yaml"))
	cfg := &config.Config{Source: source, FollowSourceLinks: true}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
		t.Fatal(err)
	}
	setupUpdatableSkill(t, source, "_dev-skills/child")
	testutil.RunGit(t, checkout, "init")
	testutil.RunGit(t, checkout, "add", ".")
	testutil.RunGit(t, checkout, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "seed")
	for _, name := range []string{"_dev-skills", "_dev-*", "_dev-skills/child"} {
		t.Run(name, func(t *testing.T) {
			output := captureStdoutStderr(t, func() {
				if err := cmdUpdate([]string{"-g", "--dry-run", name}); err != nil {
					t.Errorf("explicit update failed: %v", err)
				}
			})
			if strings.Contains(output, "not found") || !strings.Contains(output, "Dry run") {
				t.Fatalf("explicit update did not resolve %s: %s", name, output)
			}
		})
	}
	output := captureStdoutStderr(t, func() {
		if err := cmdUpdate([]string{"-g", "--all", "--dry-run"}); err != nil {
			t.Errorf("update --all: %v", err)
		}
	})
	if !strings.Contains(output, "_dev-skills: followed source link, not updated by --all") || strings.Contains(output, "would run git pull") {
		t.Fatalf("--all did not skip followed checkout: %s", output)
	}
}
