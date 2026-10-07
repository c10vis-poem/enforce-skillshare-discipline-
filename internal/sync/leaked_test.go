package sync

import (
	"path/filepath"
	"slices"
	"testing"

	"skillshare/internal/config"
)

func leakTargets(t *testing.T, scanner string) map[string]config.TargetConfig {
	dir := t.TempDir()
	return map[string]config.TargetConfig{
		scanner:  {Skills: &config.ResourceTargetConfig{Path: filepath.Join(dir, scanner)}},
		"claude": {Skills: &config.ResourceTargetConfig{Path: filepath.Join(dir, "claude")}},
	}
}

func TestLeakedSkills_ListsSkillsFilteredFromScanner(t *testing.T) {
	discovered := []DiscoveredSkill{
		{FlatName: "shared"},
		{FlatName: "claude-only", Targets: []string{"claude"}},
	}
	leaked, known := LeakedSkills("opencode", "claude", leakTargets(t, "opencode"), "merge", discovered)
	if !known || !slices.Equal(leaked, []string{"claude-only"}) {
		t.Errorf("LeakedSkills = %v, %v; want [claude-only], true", leaked, known)
	}
}

func TestLeakedSkills_NoneWhenScannerGetsEverySkill(t *testing.T) {
	discovered := []DiscoveredSkill{{FlatName: "shared"}}
	leaked, known := LeakedSkills("opencode", "claude", leakTargets(t, "opencode"), "merge", discovered)
	if !known || len(leaked) != 0 {
		t.Errorf("LeakedSkills = %v, %v; want none, true", leaked, known)
	}
}

func TestLeakedSkills_CountsSkillsStandardNamingSkips(t *testing.T) {
	// Two skills named dev collide in opencode's standard folder, so sync skips
	// both there; claude's filter keeps one, which opencode then loads from it.
	src := t.TempDir()
	discovered := []DiscoveredSkill{
		createTempSkill(t, src, "frontend/dev", "dev"),
		createTempSkill(t, src, "backend/dev", "dev"),
	}
	targets := leakTargets(t, "opencode")
	targets["opencode"].Skills.TargetNaming = "standard"
	targets["claude"].Skills.TargetNaming = "standard"
	targets["claude"].Skills.Include = []string{"frontend/dev"}

	leaked, known := LeakedSkills("opencode", "claude", targets, "merge", discovered)
	if !known || !slices.Equal(leaked, []string{"frontend/dev"}) {
		t.Errorf("LeakedSkills = %v, %v; want [frontend/dev], true", leaked, known)
	}
}

func TestLeakedSkills_UnknownForSymlinkWriter(t *testing.T) {
	// A symlink-mode folder exposes .skillignore'd skills that discovery omits.
	targets := leakTargets(t, "opencode")
	targets["claude"].Skills.Mode = "symlink"
	if _, known := LeakedSkills("opencode", "claude", targets, "merge", []DiscoveredSkill{{FlatName: "shared"}}); known {
		t.Error("want known=false for a symlink-mode writer")
	}
}

func TestLeakedSkills_UnknownForOtherRuntimes(t *testing.T) {
	discovered := []DiscoveredSkill{{FlatName: "shared"}}
	if _, known := LeakedSkills("codex", "claude", leakTargets(t, "codex"), "merge", discovered); known {
		t.Error("codex's rule for same-named skills is not known; want known=false")
	}
}
