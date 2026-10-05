package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/install"
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

func TestUpdateAllSkipsFollowedCheckoutWithMetadata(t *testing.T) {
	for _, gitFile := range []bool{false, true} {
		for _, project := range []bool{false, true} {
			for _, force := range []bool{false, true} {
				t.Run(fmt.Sprintf("gitFile=%t/project=%t/force=%t", gitFile, project, force), func(t *testing.T) {
					root := t.TempDir()
					testutil.SetIsolatedXDG(t, root)
					t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "config.yaml"))
					source := filepath.Join(root, "skills")
					cfg := &config.Config{Source: source, FollowSourceLinks: true}
					if err := cfg.Save(); err != nil {
						t.Fatal(err)
					}
					if project {
						projectCfg := &config.ProjectConfig{Sources: config.ProjectSources{Skills: source}, FollowSourceLinks: true}
						if err := projectCfg.Save(root); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.MkdirAll(source, 0755); err != nil {
						t.Fatal(err)
					}
					remote := testutil.SetupBareRemoteRepo(t, root)
					testutil.SeedRemoteBranch(t, root, remote, "main", map[string]string{"SKILL.md": "# Safe skill\n", "notes.txt": "original\n"})
					checkout := filepath.Join(root, "checkout")
					testutil.RunGit(t, "", "clone", remote, checkout)
					if gitFile {
						primary := checkout
						checkout = filepath.Join(root, "worktree")
						testutil.RunGit(t, primary, "worktree", "add", "-b", "followed", checkout)
						testutil.RunGit(t, checkout, "branch", "--set-upstream-to=origin/main")
					}
					before := testutil.RunGit(t, checkout, "rev-parse", "HEAD")
					seed := filepath.Join(root, "seed-main")
					if err := os.WriteFile(filepath.Join(seed, "SKILL.md"), []byte("# Updated skill\n"), 0644); err != nil {
						t.Fatal(err)
					}
					testutil.RunGit(t, seed, "add", ".")
					testutil.RunGit(t, seed, "commit", "-m", "advance remote")
					testutil.RunGit(t, seed, "push", "origin", "HEAD:main")
					if err := os.WriteFile(filepath.Join(checkout, "notes.txt"), []byte("local edit\n"), 0644); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
						t.Fatal(err)
					}
					store := install.LoadMetadataOrNew(source)
					store.Set("_dev-skills", &install.MetadataEntry{Source: "file://" + remote, Tracked: true})
					if err := store.Save(source); err != nil {
						t.Fatal(err)
					}
					args := []string{"--all"}
					if force {
						args = append(args, "--force")
					}
					output := captureStdoutStderr(t, func() {
						var err error
						if project {
							_, err = cmdUpdateProject(args, root)
						} else {
							err = cmdUpdate(append([]string{"-g"}, args...))
						}
						if err != nil {
							t.Errorf("update --all: %v", err)
						}
					})
					if !strings.Contains(output, "_dev-skills: followed source link, not updated by --all") {
						t.Fatalf("skip warning missing: %s", output)
					}
					if got := testutil.RunGit(t, checkout, "rev-parse", "HEAD"); got != before {
						t.Fatalf("HEAD changed: %s", got)
					}
					if got := testutil.RunGit(t, checkout, "rev-parse", "origin/main"); got != before {
						t.Fatalf("checkout fetched: %s", got)
					}
					if data, err := os.ReadFile(filepath.Join(checkout, "notes.txt")); err != nil || string(data) != "local edit\n" {
						t.Fatalf("dirty file changed: %q %v", data, err)
					}
				})
			}
		}
	}
}
