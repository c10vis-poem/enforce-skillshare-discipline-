package main

import (
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/sourcewalk"
)

func TestNormalizeUninstallName_WindowsNestedPath(t *testing.T) {
	got := normalizeUninstallNameForSeparator(`coding\_skill`, '\\')
	if got != "coding/_skill" {
		t.Fatalf("normalizeUninstallNameForSeparator() = %q, want %q", got, "coding/_skill")
	}
}

func TestNormalizeUninstallName_UnixPreservesLiteralBackslash(t *testing.T) {
	got := normalizeUninstallNameForSeparator(`coding\_skill`, '/')
	if got != `coding\_skill` {
		t.Fatalf("normalizeUninstallNameForSeparator() = %q, want %q", got, `coding\_skill`)
	}
}

// --- resolveUninstallByGlob tests ---

func TestResolveUninstallByGlob_MatchesDirs(t *testing.T) {
	src := t.TempDir()
	for _, name := range []string{"core-auth", "core-db", "utils"} {
		os.MkdirAll(filepath.Join(src, name), 0755)
	}

	targets, err := resolveUninstallByGlob("core-*", src, sourcewalk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(targets))
	}
	names := map[string]bool{}
	for _, tgt := range targets {
		names[tgt.name] = true
	}
	if !names["core-auth"] || !names["core-db"] {
		t.Errorf("expected core-auth and core-db, got %v", names)
	}
}

func TestResolveUninstallByGlob_DetectsTrackedRepos(t *testing.T) {
	src := t.TempDir()
	// Tracked repo (has .git)
	repoDir := filepath.Join(src, "_team-skills")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	// Regular skill
	os.MkdirAll(filepath.Join(src, "_team-docs"), 0755)

	targets, err := resolveUninstallByGlob("_team-*", src, sourcewalk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(targets))
	}
	for _, tgt := range targets {
		if tgt.name == "_team-skills" && !tgt.isTrackedRepo {
			t.Error("_team-skills should be detected as tracked repo")
		}
		if tgt.name == "_team-docs" && tgt.isTrackedRepo {
			t.Error("_team-docs should not be detected as tracked repo")
		}
	}
}

func TestResolveUninstallByGlob_CaseInsensitive(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "Core-Auth"), 0755)
	os.MkdirAll(filepath.Join(src, "core-db"), 0755)

	targets, err := resolveUninstallByGlob("core-*", src, sourcewalk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 case-insensitive matches, got %d", len(targets))
	}
}

func TestResolveUninstallByGlob_NoMatch(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "utils"), 0755)

	targets, err := resolveUninstallByGlob("core-*", src, sourcewalk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Errorf("expected 0 matches, got %d", len(targets))
	}
}

func TestResolveUninstallByGlob_SkipsFiles(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "core-skill"), 0755)
	os.WriteFile(filepath.Join(src, "core-file.txt"), []byte("not a dir"), 0644)

	targets, err := resolveUninstallByGlob("core-*", src, sourcewalk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 match (dirs only), got %d", len(targets))
	}
	if targets[0].name != "core-skill" {
		t.Errorf("expected core-skill, got %s", targets[0].name)
	}
}

func TestResolveUninstallTarget_TrackedNames(t *testing.T) {
	for _, tc := range []struct {
		name, input, existing, want string
		repo                        bool
	}{
		{"nested shorthand", "org/team", "", "org/_team", true},
		{"nested explicit", "org/_team", "", "org/_team", true},
		{"trailing slash", "org/team/", "", "org/_team", true},
		{"nested skill wins", "org/team", "org/team", "org/team", false},
		{"nested folder wins", "org/folder", "org/folder", "org/folder", false},
		{"top-level skill wins", "team", "team", "team", false},
		{"top-level shorthand", "team", "", "_team", true},
		{"top-level explicit", "_team", "", "_team", true},
		{"plain git checkout", "plain", "plain/.git", "plain", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := t.TempDir()
			for _, name := range []string{"_team/.git", "org/_team/.git", "org/_folder/.git", tc.existing} {
				if name != "" {
					if err := os.MkdirAll(filepath.Join(src, name), 0755); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.existing == "org/team" || tc.existing == "team" {
				if err := os.WriteFile(filepath.Join(src, tc.existing, "SKILL.md"), []byte("# Skill"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := resolveUninstallTarget(tc.input, src, "source", sourcewalk.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got.name != tc.want || got.path != filepath.Join(src, tc.want) || got.isTrackedRepo != tc.repo {
				t.Fatalf("target = %+v, want name %q, tracked %v", got, tc.want, tc.repo)
			}
		})
	}
}

func TestResolveUninstallTarget_AmbiguousNestedName(t *testing.T) {
	src := t.TempDir()
	for _, name := range []string{"a/_team/.git", "b/_team/.git"} {
		if err := os.MkdirAll(filepath.Join(src, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := resolveUninstallTarget("team", src, "source", sourcewalk.Options{}); err == nil {
		t.Fatal("ambiguous shorthand must fail")
	}
}
