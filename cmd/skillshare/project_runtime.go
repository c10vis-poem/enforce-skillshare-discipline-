package main

import (
	"slices"

	"skillshare/internal/config"
	"skillshare/internal/install"
	"skillshare/internal/sourcewalk"
)

type projectRuntime struct {
	root             string
	config           *config.ProjectConfig
	skillsStore      *install.MetadataStore
	agentsStore      *install.MetadataStore
	sourcePath       string
	agentsSourcePath string
	targets          map[string]config.TargetConfig
}

func loadProjectRuntime(root string) (*projectRuntime, error) {
	cfg, err := config.LoadProject(root)
	if err != nil {
		return nil, err
	}
	return newProjectRuntime(root, cfg, nil)
}

// newProjectRuntime builds the runtime of a loaded config. The targets in skip
// are left out of targets, so one that cannot resolve (no path) does not fail the rest.
func newProjectRuntime(root string, cfg *config.ProjectConfig, skip map[string]error) (*projectRuntime, error) {
	resolveCfg := cfg
	if len(skip) > 0 {
		c := *cfg
		c.Targets = slices.DeleteFunc(slices.Clone(cfg.Targets), func(e config.ProjectTargetEntry) bool { return skip[e.Name] != nil })
		resolveCfg = &c
	}
	targets, err := config.ResolveProjectTargets(root, resolveCfg)
	if err != nil {
		return nil, err
	}

	skillsDir := cfg.EffectiveSkillsSource(root)
	agentsDir := cfg.EffectiveAgentsSource(root)

	skillsStore, err := install.LoadMetadataWithMigration(skillsDir, "")
	if err != nil {
		return nil, err
	}

	agentsStore, err := install.LoadMetadataWithMigration(agentsDir, "agent")
	if err != nil {
		return nil, err
	}

	return &projectRuntime{
		root:             root,
		config:           cfg,
		skillsStore:      skillsStore,
		agentsStore:      agentsStore,
		sourcePath:       skillsDir,
		agentsSourcePath: agentsDir,
		targets:          targets,
	}, nil
}

// configFromProjectRuntime builds a minimal global Config from a project runtime,
// carrying host lists so that source parsing uses the project's azure_hosts / gitlab_hosts.
func configFromProjectRuntime(r *projectRuntime) *config.Config {
	return &config.Config{
		Source:            r.sourcePath,
		AgentsSource:      r.agentsSourcePath,
		FollowSourceLinks: r.config.FollowSourceLinks,
		Ignore:            r.config.Ignore,
		GitLabHosts:       r.config.GitLabHosts,
		AzureHosts:        r.config.AzureHosts,
	}
}

// skillsWalk returns a fresh traversal policy for one operation on the
// project's skills source.
func (r *projectRuntime) skillsWalk() sourcewalk.Options {
	return config.SkillsWalk(r.config.FollowSourceLinks, r.sourcePath, r.targets)
}
