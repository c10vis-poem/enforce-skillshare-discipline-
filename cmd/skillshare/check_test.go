package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/testutil"
)

func TestURLBranchKey_RoundTrip(t *testing.T) {
	tests := []struct {
		url    string
		branch string
	}{
		{"https://github.com/owner/repo", "develop"},
		{"https://github.com/owner/repo", ""},
		{"git@github.com:org/repo.git", ""},
		{"git@github.com:org/repo.git", "feature"},
		{"https://gitlab.com/group/subgroup/project.git", "main"},
		{"file:///tmp/repo.git", "dev"},
	}

	for _, tt := range tests {
		key := urlBranchKey(tt.url, tt.branch)
		gotURL, gotBranch := splitURLBranch(key)
		if gotURL != tt.url {
			t.Errorf("urlBranchKey(%q, %q) → splitURLBranch: URL = %q, want %q", tt.url, tt.branch, gotURL, tt.url)
		}
		if gotBranch != tt.branch {
			t.Errorf("urlBranchKey(%q, %q) → splitURLBranch: Branch = %q, want %q", tt.url, tt.branch, gotBranch, tt.branch)
		}
	}
}

func TestSplitURLBranch_SSHURLWithoutBranch(t *testing.T) {
	// Regression: "@" in SSH URLs must not be confused with the separator.
	url, branch := splitURLBranch("git@github.com:org/repo.git")
	if url != "git@github.com:org/repo.git" {
		t.Errorf("URL = %q, want full SSH URL", url)
	}
	if branch != "" {
		t.Errorf("Branch = %q, want empty", branch)
	}
}

func TestCheckFollowedCheckoutResolution(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(fmt.Sprintf("project=%t", project), func(t *testing.T) {
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
			testutil.SeedRemoteBranch(t, root, remote, "main", map[string]string{"SKILL.md": "# Safe skill\n"})
			checkout := filepath.Join(root, "checkout")
			testutil.RunGit(t, "", "clone", remote, checkout)
			if err := os.Symlink(checkout, filepath.Join(source, "_dev-skills")); err != nil {
				t.Fatal(err)
			}
			for _, names := range [][]string{nil, {"_dev-skills"}} {
				output := captureStdoutStderr(t, func() {
					var err error
					if project {
						err = cmdCheckProject(root, &checkOptions{names: names, json: true})
					} else {
						err = cmdCheck(append([]string{"-g", "--json"}, names...))
					}
					if err != nil {
						t.Errorf("check: %v", err)
					}
				})
				var result checkOutput
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("invalid check output: %v: %s", err, output)
				}
				if len(result.TrackedRepos) != 1 || result.TrackedRepos[0].Name != "_dev-skills" || result.TrackedRepos[0].Status != "up_to_date" {
					t.Fatalf("check %v did not resolve followed checkout: %s", names, output)
				}
			}
		})
	}
}
