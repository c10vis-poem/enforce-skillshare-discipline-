package instructions

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"skillshare/internal/config"
	syncpkg "skillshare/internal/sync"
)

// Target is a target's global instruction file, the unit shared files attach to.
type Target struct {
	Name   string
	File   string // absolute path
	Import bool   // several shared files can attach, each as an @path line
}

// Resolver locates the files of extras: the source directory of an extra and
// the directory a configured target path means.
type Resolver struct {
	SourceDir func(config.ExtraConfig) string
	TargetDir func(string) string
}

// IsShared reports whether extra is a shared instruction file: a single-file
// extra whose file is AGENTS.md. Other single-file extras may target the same
// instruction files, but Assign and PlanAssign never attach or detach them.
func IsShared(extra config.ExtraConfig) bool { return extra.File == AgentsFile }

// Shared returns the shared instruction files among extras.
func Shared(extras []config.ExtraConfig) []config.ExtraConfig {
	return slices.DeleteFunc(slices.Clone(extras), func(e config.ExtraConfig) bool { return !IsShared(e) })
}

// Assign makes want exactly the shared files attached to t, in that order of
// addition. Files no longer wanted are restored first (their link or import
// line removed, the replaced file put back); newly wanted ones are added to
// the extra's targets and synced. A target without import takes at most one.
// It returns the updated extras; on error the extras reflect the steps done.
func Assign(extras []config.ExtraConfig, t Target, want []string, r Resolver, warnings ...*[]syncpkg.FileWarning) ([]config.ExtraConfig, error) {
	planned, err := PlanAssign(extras, t, want, r)
	if err != nil {
		return extras, err
	}

	var errs []string
	for i := range extras {
		if !IsShared(extras[i]) || slices.Contains(want, extras[i].Name) {
			continue
		}
		j := targetIndex(extras[i], t.File, r)
		if j == -1 {
			continue
		}
		tc := extras[i].Targets[j]
		f := syncpkg.NewExtraFile(r.SourceDir(extras[i]), extras[i].File, r.TargetDir(tc.Path), tc.As, tc.Mode)
		if _, err := syncpkg.RestoreExtraTarget(f); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		extras[i].Targets = slices.Delete(extras[i].Targets, j, j+1)
	}
	if len(errs) > 0 {
		return extras, fmt.Errorf("could not restore %s: %s", t.Name, strings.Join(errs, "; "))
	}

	for _, name := range want {
		i := indexOf(extras, name)
		if targetIndex(extras[i], t.File, r) != -1 {
			continue
		}
		// The mode the plan validated, chosen with every wanted file in view;
		// recomputing it here, after the removals above, could pick another.
		pi := indexOf(planned, name)
		tc := planned[pi].Targets[targetIndex(planned[pi], t.File, r)]
		if tc.As == extras[i].File {
			tc.As = ""
		}
		if err := config.ValidateExtraConfig(config.ExtraConfig{Name: name, File: extras[i].File, Targets: []config.ExtraTargetConfig{tc}}); err != nil {
			return extras, err
		}
		f := syncpkg.NewExtraFile(r.SourceDir(extras[i]), extras[i].File, r.TargetDir(tc.Path), tc.As, tc.Mode)
		result, err := syncpkg.SyncExtraFile(f, false, "")
		if err != nil {
			return extras, fmt.Errorf("attach %s to %s: %w", name, t.Name, err)
		}
		if len(warnings) > 0 {
			*warnings[0] = append(*warnings[0], result.FileWarnings...)
		}
		if result.Skipped > 0 {
			return extras, fmt.Errorf("%s", strings.Join(result.Warnings, "; "))
		}
		extras[i].Targets = append(extras[i].Targets, tc)
	}
	return extras, nil
}

// PlanAssign validates the complete desired ownership without changing files or
// the caller's config. API batches use it before making their first mutation.
func PlanAssign(extras []config.ExtraConfig, t Target, want []string, r Resolver) ([]config.ExtraConfig, error) {
	for _, name := range want {
		if i := indexOf(extras, name); i == -1 || !IsShared(extras[i]) {
			return nil, fmt.Errorf("shared instruction file %q not found", name)
		}
	}
	// Detach first, so the defaults below see only the files that stay.
	next := slices.Clone(extras)
	for i := range next {
		next[i].Targets = slices.Clone(next[i].Targets)
		if !IsShared(next[i]) || slices.Contains(want, next[i].Name) {
			continue
		}
		if j := targetIndex(next[i], t.File, r); j != -1 {
			next[i].Targets = slices.Delete(next[i].Targets, j, j+1)
		}
	}
	if !t.Import && len(want) > 1 && !holdsManaged(next, t, r) {
		return nil, &config.ExtraTargetConflict{Name: want[0], Target: t.Name}
	}
	for _, name := range want {
		i := indexOf(next, name)
		if targetIndex(next[i], t.File, r) != -1 {
			continue
		}
		tc := config.ExtraTargetConfig{Path: filepath.Dir(t.File), As: filepath.Base(t.File), Mode: defaultAssignMode(t, next, r)}
		next[i].Targets = append(next[i].Targets, tc)
	}
	if err := config.ValidateExtraConnections(next, r.SourceDir, r.TargetDir); err != nil {
		return nil, err
	}
	return next, nil
}

// holdsManaged reports whether the target's file already holds a shared file in
// a managed block, which lets it take several.
func holdsManaged(extras []config.ExtraConfig, t Target, r Resolver) bool {
	return defaultAssignMode(Target{Name: t.Name, File: t.File}, extras, r) == "prepend"
}

// defaultAssignMode is how a newly connected target gets a shared file: one
// @path line where the tool reads them; otherwise a link, unless the target's
// file already holds another shared file in a managed block, in which case the
// new one goes in a block of its own (a link would replace the file). A target
// without @import is switched to prepend or append from the mode picker.
func defaultAssignMode(t Target, extras []config.ExtraConfig, r Resolver) string {
	if t.Import {
		return "import"
	}
	for _, e := range extras {
		if !IsShared(e) {
			continue
		}
		if j := targetIndex(e, t.File, r); j != -1 && config.ManagedExtraMode(e.Targets[j].Mode) {
			return "prepend"
		}
	}
	return "symlink"
}

// Find returns the index of the named single-file extra's target that writes
// file, or -1.
func Find(extras []config.ExtraConfig, name, file string, r Resolver) (int, int) {
	i := indexOf(extras, name)
	if i == -1 || extras[i].File == "" {
		return -1, -1
	}
	return i, targetIndex(extras[i], file, r)
}

// ExtraFile returns the sync view of an extra's j-th target.
func ExtraFile(extra config.ExtraConfig, j int, r Resolver) syncpkg.ExtraFile {
	tc := extra.Targets[j]
	return syncpkg.NewExtraFile(r.SourceDir(extra), extra.File, r.TargetDir(tc.Path), tc.As, tc.Mode)
}

func indexOf(extras []config.ExtraConfig, name string) int {
	for i := range extras {
		if extras[i].Name == name {
			return i
		}
	}
	return -1
}

func targetIndex(extra config.ExtraConfig, file string, r Resolver) int {
	for j := range extra.Targets {
		if samePath(ExtraFile(extra, j, r).Target, file) {
			return j
		}
	}
	return -1
}
