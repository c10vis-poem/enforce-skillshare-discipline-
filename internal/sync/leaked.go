package sync

import (
	"sort"

	"skillshare/internal/config"
)

// ownFolderLast lists targets whose runtime reads the folders it also scans
// first and its own skills folder last, keeping the last skill of each name.
// A skill synced to both folders then loads once, from the target's own copy.
// OpenCode: packages/opencode/src/skill/index.ts in anomalyco/opencode.
var ownFolderLast = map[string]bool{"opencode": true}

// LeakedSkills returns the source skills scanner's runtime loads from writer's
// folder although scanner's own filters leave them out. known is false when
// that cannot be told (the runtime's rule for same-named skills is unknown, a
// filter is invalid, or nothing was discovered); every skill in writer's
// folder may then reach scanner.
func LeakedSkills(scanner, writer string, targets map[string]config.TargetConfig, defaultMode, sourcePath string, discovered []DiscoveredSkill) (leaked []string, known bool) {
	if !ownFolderLast[scanner] || discovered == nil {
		return nil, false
	}
	own, err := TargetSkills(scanner, targets[scanner], defaultMode, sourcePath, discovered)
	if err != nil {
		return nil, false
	}
	theirs, err := TargetSkills(writer, targets[writer], defaultMode, sourcePath, discovered)
	if err != nil {
		return nil, false
	}

	loaded := make(map[string]bool, len(own))
	for _, s := range own {
		loaded[s.FlatName] = true
	}
	// Skills the user put in writer's folder by hand are not skillshare's call.
	fromSource := make(map[string]bool, len(discovered))
	for _, s := range discovered {
		fromSource[s.FlatName] = true
	}
	for _, s := range theirs {
		if fromSource[s.FlatName] && !loaded[s.FlatName] {
			leaked = append(leaked, s.FlatName)
		}
	}
	sort.Strings(leaked)
	return leaked, true
}

// HarmlessOverlap returns a check for config.DetectPathOverlap: scanner reading
// writer's folder is harmless when it loads nothing its own filters leave out.
func HarmlessOverlap(targets map[string]config.TargetConfig, defaultMode, sourcePath string, discovered []DiscoveredSkill) func(scanner, writer string) bool {
	return func(scanner, writer string) bool {
		leaked, known := LeakedSkills(scanner, writer, targets, defaultMode, sourcePath, discovered)
		return known && len(leaked) == 0
	}
}
