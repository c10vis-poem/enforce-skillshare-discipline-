package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/sourcefs"
	"skillshare/internal/trash"
)

func TestTrashRestoreFollowPolicyAcrossModes(t *testing.T) {
	for _, mode := range []runMode{modeGlobal, modeProject} {
		for _, follow := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode=%v/follow=%t", mode, follow), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", root)
				t.Setenv("XDG_DATA_HOME", root)
				t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "config.yaml"))
				cfg := &config.Config{Source: filepath.Join(root, "skills"), FollowSourceLinks: follow}
				if err := cfg.Save(); err != nil {
					t.Fatal(err)
				}
				if mode == modeProject {
					projectCfg := &config.ProjectConfig{FollowSourceLinks: follow}
					if err := projectCfg.Save(root); err != nil {
						t.Fatal(err)
					}
				}
				source, err := resolveSourceDir(mode, root, kindSkills)
				if err != nil {
					t.Fatal(err)
				}
				checkout := filepath.Join(root, "checkout")
				for _, dir := range []string{source, filepath.Join(checkout, "foo")} {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
					t.Skip(err)
				}
				trashBase := resolveTrashBase(mode, root, kindSkills)
				trashed, err := trash.MoveToTrash(filepath.Join(checkout, "foo"), "_dev-skills/foo", trashBase)
				if err != nil {
					t.Fatal(err)
				}
				err = trashRestore(mode, root, []string{"_dev-skills/foo"}, kindSkills)
				if follow {
					if err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(filepath.Join(checkout, "foo")); err != nil {
						t.Fatalf("skill not restored to checkout: %v", err)
					}
				} else {
					if !errors.Is(err, sourcefs.ErrLink) {
						t.Fatalf("expected disabled-policy refusal, got %v", err)
					}
					if _, err := os.Stat(trashed); err != nil {
						t.Fatalf("trash copy missing: %v", err)
					}
				}
			})
		}
	}
}
