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
	leaked, known := LeakedSkills("opencode", "claude", leakTargets(t, "opencode"), "merge", t.TempDir(), discovered)
	if !known || !slices.Equal(leaked, []string{"claude-only"}) {
		t.Errorf("LeakedSkills = %v, %v; want [claude-only], true", leaked, known)
	}
}

func TestLeakedSkills_NoneWhenScannerGetsEverySkill(t *testing.T) {
	discovered := []DiscoveredSkill{{FlatName: "shared"}}
	leaked, known := LeakedSkills("opencode", "claude", leakTargets(t, "opencode"), "merge", t.TempDir(), discovered)
	if !known || len(leaked) != 0 {
		t.Errorf("LeakedSkills = %v, %v; want none, true", leaked, known)
	}
}

func TestLeakedSkills_UnknownForOtherRuntimes(t *testing.T) {
	discovered := []DiscoveredSkill{{FlatName: "shared"}}
	if _, known := LeakedSkills("codex", "claude", leakTargets(t, "codex"), "merge", t.TempDir(), discovered); known {
		t.Error("codex's rule for same-named skills is not known; want known=false")
	}
}
