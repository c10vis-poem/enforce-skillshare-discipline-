package sync

import (
	"sort"

	"skillshare/internal/config"
)

// sameNameLoadsOnce lists targets whose runtime keeps one skill per name across
// its own folder and the folders it also scans. Which copy wins is not fixed
// (OpenCode loads them concurrently: packages/opencode/src/skill/index.ts in
// anomalyco/opencode), but a source skill synced to both folders has the same
// content either way, so it loads once as intended.
var sameNameLoadsOnce = map[string]bool{"opencode": true}

// LeakedSkills returns the source skills scanner's runtime loads from writer's
// folder although sync leaves them out of scanner's own folder. known is false
// when that cannot be told (the runtime's rule for same-named skills is
// unknown, a filter is invalid, or nothing was discovered); every skill in
// writer's folder may then reach scanner.
func LeakedSkills(scanner, writer string, targets map[string]config.TargetConfig, defaultMode string, discovered []DiscoveredSkill) (leaked []string, known bool) {
	if !sameNameLoadsOnce[scanner] || discovered == nil {
		return nil, false
	}
	own, err := syncedSkills(scanner, targets[scanner], defaultMode, discovered)
	if err != nil {
		return nil, false
	}
	theirs, err := syncedSkills(writer, targets[writer], defaultMode, discovered)
	if err != nil {
		return nil, false
	}

	synced := make(map[string]bool, len(own))
	for _, s := range own {
		synced[s.FlatName] = true
	}
	for _, s := range theirs {
		if !synced[s.FlatName] {
			leaked = append(leaked, s.FlatName)
		}
	}
	sort.Strings(leaked)
	return leaked, true
}

// syncedSkills returns the source skills sync puts in a target's folder: the
// whole source in symlink mode, otherwise what its filters and target_naming
// let through.
func syncedSkills(name string, target config.TargetConfig, defaultMode string, discovered []DiscoveredSkill) ([]DiscoveredSkill, error) {
	sc := target.SkillsConfig()
	mode := sc.Mode
	if mode == "" {
		mode = defaultMode
	}
	if mode == "symlink" {
		return discovered, nil
	}
	res, err := ResolveTargetSkillsForTarget(name, sc, discovered)
	if err != nil {
		return nil, err
	}
	skills := make([]DiscoveredSkill, len(res.Skills))
	for i, r := range res.Skills {
		skills[i] = r.Skill
	}
	return skills, nil
}

// HarmlessOverlap returns a check for config.DetectPathOverlap: scanner reading
// writer's folder is harmless when it loads nothing sync leaves out of its own.
func HarmlessOverlap(targets map[string]config.TargetConfig, defaultMode string, discovered []DiscoveredSkill) func(scanner, writer string) bool {
	return func(scanner, writer string) bool {
		leaked, known := LeakedSkills(scanner, writer, targets, defaultMode, discovered)
		return known && len(leaked) == 0
	}
}
