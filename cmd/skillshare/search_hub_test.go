package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/hub"
)

func TestLooksLikeURLOrPath_SSH(t *testing.T) {
	cases := map[string]bool{
		"git@github.com:owner/repo.git":                 true,
		"git@ghe.corp.com:team/skills.git//hubs/h.json": true,
		"ssh://git@host/org/repo.git":                   true,
		"https://internal.corp/hub.json":                true,
		"./skillshare-hub.json":                         true,
		"team":                                          false,
		"my-hub":                                        false,
	}
	for in, want := range cases {
		if got := looksLikeURLOrPath(in); got != want {
			t.Errorf("looksLikeURLOrPath(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHubIndexFollowedSourceLinks(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, follow := range []bool{false, true} {
			for _, explicit := range []bool{false, true} {
				t.Run(fmt.Sprintf("project=%t/follow=%t/explicit=%t", project, follow, explicit), func(t *testing.T) {
					root := t.TempDir()
					t.Chdir(root)
					t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "config.yaml"))
					source := filepath.Join(root, "skills")
					cfg := &config.Config{Source: source, FollowSourceLinks: follow}
					if err := cfg.Save(); err != nil {
						t.Fatal(err)
					}
					if project {
						cfg := &config.ProjectConfig{Sources: config.ProjectSources{Skills: source}, FollowSourceLinks: follow}
						if err := cfg.Save(root); err != nil {
							t.Fatal(err)
						}
					}
					if explicit {
						source = filepath.Join(root, "custom-source")
					}
					checkout := t.TempDir()
					for _, dir := range []string{source, filepath.Join(checkout, "linked-skill")} {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.WriteFile(filepath.Join(checkout, "linked-skill", "SKILL.md"), []byte("# Linked skill\n"), 0644); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(checkout, filepath.Join(source, "linked")); err != nil {
						t.Fatal(err)
					}
					output := filepath.Join(root, "index.json")
					args := []string{"-g", "-o", output}
					if project {
						args[0] = "-p"
					}
					if explicit {
						args = append(args, "--source", source)
					}
					captureStdoutStderr(t, func() {
						if err := cmdHubIndex(args); err != nil {
							t.Errorf("hub index: %v", err)
						}
					})
					data, err := os.ReadFile(output)
					if err != nil {
						t.Fatal(err)
					}
					var index hub.Index
					if err := json.Unmarshal(data, &index); err != nil {
						t.Fatal(err)
					}
					want := 0
					if follow {
						want = 1
					}
					if len(index.Skills) != want || (follow && index.Skills[0].Name != "linked-skill") {
						t.Fatalf("follow=%t index=%s", follow, data)
					}
				})
			}
		}
	}
}

func TestHubIndexExplicitSourceWithoutConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "missing.yaml"))
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	captureStdoutStderr(t, func() {
		if err := cmdHubIndex([]string{"-g", "--source", root}); err != nil {
			t.Errorf("explicit source requires config: %v", err)
		}
	})
	if _, err := os.Stat(filepath.Join(root, "skillshare-hub.json")); err != nil {
		t.Fatal(err)
	}
}
