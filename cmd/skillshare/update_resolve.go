package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"skillshare/internal/install"
	"skillshare/internal/sourcewalk"
	"skillshare/internal/utils"
)

type updateTarget struct {
	name   string                 // relative path from source dir (display name)
	path   string                 // absolute path on disk
	isRepo bool                   // true for tracked repos (_-prefixed git repos)
	meta   *install.MetadataEntry // cached metadata; nil for tracked repos
}

// resolveByBasename searches nested skills and tracked repos by their
// directory basename. Returns an error when zero or multiple matches found.
func resolveByBasename(sourceDir, name string, walks ...sourcewalk.Options) (updateTarget, error) {
	var matches []updateTarget

	// Search tracked repos
	repos, _ := install.GetTrackedRepos(sourceDir, walks...)
	for _, r := range install.MatchTrackedRepos(repos, name) {
		matches = append(matches, updateTarget{name: r, path: filepath.Join(sourceDir, r), isRepo: true})
	}

	// Search updatable skills
	skills, _ := install.GetUpdatableSkills(sourceDir)
	for _, s := range skills {
		if s == name || filepath.Base(s) == name {
			matches = append(matches, updateTarget{name: s, path: filepath.Join(sourceDir, s), isRepo: false})
		}
	}

	if len(matches) == 0 {
		return updateTarget{}, fmt.Errorf("'%s' not found as tracked repo or skill with metadata", name)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	// Ambiguous: list all matches
	lines := []string{fmt.Sprintf("'%s' matches multiple items:", name)}
	for _, m := range matches {
		lines = append(lines, fmt.Sprintf("  - %s", m.name))
	}
	lines = append(lines, "Please specify the full path")
	return updateTarget{}, fmt.Errorf("%s", strings.Join(lines, "\n"))
}

// resolveByGlob searches tracked repos and updatable skills whose basenames
// match the given glob pattern (e.g. "core-*", "_team-?"). Returns all matches
// sorted by name.
func resolveByGlob(sourceDir, pattern string, walks ...sourcewalk.Options) ([]updateTarget, error) {
	var matches []updateTarget

	repos, _ := install.GetTrackedRepos(sourceDir, walks...)
	for _, r := range repos {
		if matchGlob(pattern, filepath.Base(r)) {
			matches = append(matches, updateTarget{name: r, path: filepath.Join(sourceDir, r), isRepo: true})
		}
	}

	skills, _ := install.GetUpdatableSkills(sourceDir)
	for _, s := range skills {
		if matchGlob(pattern, filepath.Base(s)) {
			matches = append(matches, updateTarget{name: s, path: filepath.Join(sourceDir, s), isRepo: false})
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].name < matches[j].name })
	return matches, nil
}

// resolveGroupUpdatable finds all updatable items (tracked repos or skills with
// metadata) under a group directory. Local skills without metadata are skipped.
func resolveGroupUpdatable(group, sourceDir string, walks ...sourcewalk.Options) ([]updateTarget, error) {
	group = strings.TrimSuffix(group, "/")
	groupPath := filepath.Join(sourceDir, group)

	info, err := os.Stat(groupPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("group '%s' not found in source", group)
	}

	walkRoot, logicalRoot, err := resolveGroupWalk(groupPath, sourceDir, walks)
	if err != nil {
		return nil, fmt.Errorf("group '%s' resolves outside source directory", group)
	}
	resolvedSourceDir := utils.ResolveSymlink(sourceDir)

	// Load store once before walk (not per iteration)
	store, _ := install.LoadMetadata(resolvedSourceDir)

	var matches []updateTarget
	if walkErr := filepath.Walk(walkRoot, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if path == walkRoot || !fi.IsDir() {
			return nil
		}
		if fi.Name() == ".git" {
			return filepath.SkipDir
		}

		tail, err := filepath.Rel(walkRoot, path)
		if err != nil {
			return err
		}
		logicalPath := filepath.Join(logicalRoot, tail)

		rel, relErr := filepath.Rel(resolvedSourceDir, logicalPath)
		if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return nil
		}

		// Tracked repo (_-prefixed checkout)
		if install.IsTrackedCheckout(path) {
			matches = append(matches, updateTarget{name: rel, path: logicalPath, isRepo: true})
			return filepath.SkipDir
		}

		// Skill with metadata (centralized store)
		if entry := store.GetByPath(rel); entry != nil && entry.Source != "" {
			matches = append(matches, updateTarget{name: rel, path: logicalPath, isRepo: false, meta: entry})
			return filepath.SkipDir
		}

		return nil
	}); walkErr != nil {
		return nil, fmt.Errorf("failed to walk group '%s': %w", group, walkErr)
	}

	return matches, nil
}

// isGroupDir checks if a name corresponds to a group directory (a container
// for other skills). Returns false for tracked repos, skills with metadata,
// and directories that are themselves a skill (have SKILL.md).
func isGroupDir(name, sourceDir string, store *install.MetadataStore) bool {
	path := filepath.Join(sourceDir, name)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	// Not a tracked repo
	if install.IsTrackedCheckout(path) {
		return false
	}
	// Not a skill with metadata
	if entry := store.Get(name); entry != nil && entry.Source != "" {
		return false
	}
	// Not a skill directory (has SKILL.md)
	if _, statErr := os.Stat(filepath.Join(path, "SKILL.md")); statErr == nil {
		return false
	}
	return true
}

// resolveGroupWalk permits an external group only when the operation's policy
// validates that first-level source link. Nested group links retain the guard.
func resolveGroupWalk(groupPath, sourceDir string, walks []sourcewalk.Options) (physical, logical string, err error) {
	physical = utils.ResolveSymlink(groupPath)
	source := utils.ResolveSymlink(sourceDir)
	if len(walks) > 0 && utils.PathsEqual(utils.ResolveSymlink(filepath.Dir(groupPath)), source) {
		if resolved, ok := walks[0].Follow.Resolve(groupPath); ok {
			return resolved, filepath.Join(source, filepath.Base(groupPath)), nil
		}
	}
	if rel, relErr := filepath.Rel(source, physical); relErr != nil || strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("group resolves outside source directory")
	}
	return physical, physical, nil
}
