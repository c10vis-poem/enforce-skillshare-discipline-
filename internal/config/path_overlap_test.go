package config

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestDetectPathOverlap_CrossRuntimeDiscovery(t *testing.T) {
	// codex's runtime reads ~/.agents/skills, which universal writes to.
	involved := DetectPathOverlap(map[string]TargetConfig{
		"codex":     {Skills: &ResourceTargetConfig{Path: "~/.codex/skills"}},
		"universal": {Skills: &ResourceTargetConfig{Path: "~/.agents/skills"}},
	}, false, nil)

	for _, want := range []string{"codex", "universal"} {
		if !slices.Contains(involved, want) {
			t.Errorf("DetectPathOverlap = %v, missing %s", involved, want)
		}
	}
}

func TestDetectPathOverlap_CodexSharesUniversalPath(t *testing.T) {
	// Default codex path is universal's ~/.agents/skills — a shared-primary overlap.
	involved := DetectPathOverlap(map[string]TargetConfig{
		"codex":     {Skills: &ResourceTargetConfig{Path: "~/.agents/skills"}},
		"universal": {Skills: &ResourceTargetConfig{Path: "~/.agents/skills"}},
	}, false, nil)

	for _, want := range []string{"codex", "universal"} {
		if !slices.Contains(involved, want) {
			t.Errorf("DetectPathOverlap = %v, missing %s", involved, want)
		}
	}
}

func TestDetectPathOverlap_LegacyGoosePathStillWarns(t *testing.T) {
	// A config written before the default moved keeps goose on its own path;
	// goose still reads universal's ~/.agents/skills, so the warning must stay.
	involved := DetectPathOverlap(map[string]TargetConfig{
		"goose":     {Skills: &ResourceTargetConfig{Path: "~/.config/goose/skills"}},
		"universal": {Skills: &ResourceTargetConfig{Path: "~/.agents/skills"}},
	}, false, nil)

	for _, want := range []string{"goose", "universal"} {
		if !slices.Contains(involved, want) {
			t.Errorf("DetectPathOverlap = %v, missing %s", involved, want)
		}
	}
}

func TestDetectPathOverlap_IgnoresScannerOwnPath(t *testing.T) {
	// claude's runtime scans its own ~/.claude/skills; that is not an overlap
	// with the unrelated target writing elsewhere.
	involved := DetectPathOverlap(map[string]TargetConfig{
		"claude": {Skills: &ResourceTargetConfig{Path: "~/.claude/skills"}},
		"junie":  {Skills: &ResourceTargetConfig{Path: "~/.junie/skills"}},
	}, false, nil)

	if len(involved) != 0 {
		t.Errorf("expected no overlap, got %v", involved)
	}
}

func TestSkillsFolderConflicts_DifferentFilters(t *testing.T) {
	got := SkillsFolderConflicts(map[string]TargetConfig{
		"universal": {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills", Exclude: []string{"feature-radar*"}}},
		"codex":     {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills/"}},
		"claude":    {Skills: &ResourceTargetConfig{Path: "/tmp/claude/skills"}},
	})
	want := []SkillsFolderConflict{{Path: "/tmp/agents/skills", Targets: []string{"codex", "universal"}, Keep: "universal", Stop: []string{"codex"}}}
	if !slices.EqualFunc(got, want, conflictEqual) {
		t.Errorf("SkillsFolderConflicts = %+v, want %+v", got, want)
	}
}

func TestSkillsFolderConflicts_SameSettingsNoConflict(t *testing.T) {
	got := SkillsFolderConflicts(map[string]TargetConfig{
		"universal": {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills", Include: []string{"a", "b"}}},
		"codex":     {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills", Include: []string{"b", "a"}}},
	})
	if len(got) != 0 {
		t.Errorf("expected no conflict, got %+v", got)
	}
}

func TestSkillsFolderConflicts_DifferentFiltersKeepsFirstName(t *testing.T) {
	got := SkillsFolderConflicts(map[string]TargetConfig{
		"warp":  {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills", Include: []string{"x*"}}},
		"amp":   {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills"}},
		"witsy": {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills"}},
	})
	want := []SkillsFolderConflict{{Path: "/tmp/agents/skills", Targets: []string{"amp", "warp", "witsy"}, Keep: "amp", Stop: []string{"warp", "witsy"}}}
	if !slices.EqualFunc(got, want, conflictEqual) {
		t.Errorf("SkillsFolderConflicts = %+v, want %+v", got, want)
	}
}

func TestSkillsFolderConflicts_IgnoresSkillsOff(t *testing.T) {
	got := SkillsFolderConflicts(map[string]TargetConfig{
		"universal": {Skills: &ResourceTargetConfig{Path: "/tmp/agents/skills", Exclude: []string{"x"}}},
		"codex":     disabledAt("/tmp/agents/skills"),
	})
	if len(got) != 0 {
		t.Errorf("expected no conflict with skills off, got %+v", got)
	}
}

func conflictEqual(a, b SkillsFolderConflict) bool {
	return filepath.ToSlash(a.Path) == filepath.ToSlash(b.Path) && a.Keep == b.Keep && slices.Equal(a.Targets, b.Targets) && slices.Equal(a.Stop, b.Stop)
}
