package main

import (
	"skillshare/internal/config"
	"skillshare/internal/sourcewalk"
)

// projectSkillsWalk returns a fresh traversal policy for one operation on a
// project's skills source, for commands that do not load a projectRuntime.
func projectSkillsWalk(root string, cfg *config.ProjectConfig) sourcewalk.Options {
	targets, _ := config.ResolveValidProjectTargets(root, cfg)
	return config.SkillsWalk(cfg.FollowSourceLinks, cfg.EffectiveSkillsSource(root), targets)
}
