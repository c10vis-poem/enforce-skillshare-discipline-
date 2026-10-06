package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"skillshare/internal/config"
	"skillshare/internal/projectdir"
	ssync "skillshare/internal/sync"
)

// projectResource is one resource kind of a project. nil means it is not sent.
type projectResource struct {
	Mode         string   `json:"mode"`
	TargetNaming string   `json:"targetNaming,omitempty"`
	Include      []string `json:"include"`
	Exclude      []string `json:"exclude"`
}

type projectGroup struct {
	Target     string   `json:"target"` // target name, as the sync and target lists show it
	Tools      []string `json:"tools"`
	SkillsPath string   `json:"skillsPath"`
	AgentsPath string   `json:"agentsPath"`
}

type projectItem struct {
	Root         string           `json:"root"` // as written in config.yaml
	Path         string           `json:"path"` // absolute
	Name         string           `json:"name"`
	CustomName   string           `json:"customName,omitempty"`
	Targets      []string         `json:"targets"`
	Skills       *projectResource `json:"skills"`
	Agents       *projectResource `json:"agents"`
	Groups       []projectGroup   `json:"groups"`
	Missing      bool             `json:"missing"`
	HasOwnConfig bool             `json:"hasOwnConfig"`
}

type projectBody struct {
	Root    string           `json:"root"`
	Name    string           `json:"name"`
	Targets []string         `json:"targets"`
	Skills  *projectResource `json:"skills"`
	Agents  *projectResource `json:"agents"`
	// Create refuses a root that is already declared.
	Create bool `json:"create"`
}

func toProjectResource(rc *config.ResourceTargetConfig) *projectResource {
	if rc == nil {
		return nil
	}
	return &projectResource{Mode: rc.Mode, TargetNaming: rc.TargetNaming, Include: append([]string{}, rc.Include...), Exclude: append([]string{}, rc.Exclude...)}
}

func (r *projectResource) config() *config.ResourceTargetConfig {
	if r == nil {
		return nil
	}
	return &config.ResourceTargetConfig{Mode: r.Mode, TargetNaming: r.TargetNaming, Include: r.Include, Exclude: r.Exclude}
}

// hasOwnProjectConfig reports a folder that project mode manages too. Sync never
// overwrites what it does not own there, so the dashboard says so up front.
func hasOwnProjectConfig(root string) bool {
	_, ok := projectdir.Find(root)
	return ok
}

func (s *Server) requireGlobalProjects(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.IsProjectMode() {
			writeError(w, http.StatusBadRequest, "projects belong in the global config")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := []projectItem{}
	for root, project := range s.cfg.Projects {
		abs := config.ExpandPath(root)
		name := project.ProjectName(root)
		item := projectItem{
			Root: root, Path: abs, Name: name, CustomName: project.Name,
			Targets: append([]string{}, project.Targets...),
			Skills:  toProjectResource(project.Skills), Agents: toProjectResource(project.Agents),
			Groups:       []projectGroup{},
			HasOwnConfig: hasOwnProjectConfig(abs),
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			item.Missing = true
		}
		groups, _ := project.ToolGroups(root) // Load has already accepted every target
		for _, group := range groups {
			item.Groups = append(item.Groups, projectGroup{Target: config.ProjectTargetName(name, group.Target()), Tools: group.Tools, SkillsPath: group.SkillsPath, AgentsPath: group.AgentsPath})
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	type convertible struct {
		Root    string   `json:"root"`
		Targets []string `json:"targets"`
		Tools   []string `json:"tools"`
		Agents  bool     `json:"agents"`
	}
	offers := []convertible{}
	for _, found := range s.cfg.ConvertibleProjects() {
		offers = append(offers, convertible{Root: found.Root, Targets: found.Targets, Tools: found.Project.Targets, Agents: found.Project.Agents != nil})
	}

	type tool struct {
		Name       string `json:"name"`
		SkillsPath string `json:"skillsPath"`
		AgentsPath string `json:"agentsPath"`
	}
	tools := []tool{}
	agentPaths := config.ProjectAgentTargets()
	for name, target := range config.ProjectTargets() {
		tools = append(tools, tool{Name: name, SkillsPath: target.Path, AgentsPath: agentPaths[name].Path})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	writeJSON(w, map[string]any{"projects": items, "convertible": offers, "tools": tools})
}

// handleSaveProject adds a project or replaces the one declared under the same root.
func (s *Server) handleSaveProject(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var body projectBody
	if err := decodeJSON(w, r, &body, defaultJSONBodyLimit); err != nil {
		if !errors.Is(err, errBodyTooLarge) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return
	}
	if body.Root == "" {
		writeError(w, http.StatusBadRequest, "root is required")
		return
	}
	for _, rc := range []*projectResource{body.Skills, body.Agents} {
		if rc == nil {
			continue
		}
		if _, err := ssync.FilterSkills(nil, rc.Include, rc.Exclude); err != nil {
			writeError(w, http.StatusBadRequest, "invalid include/exclude pattern: "+err.Error())
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	root := s.declaredProjectRoot(body.Root)
	_, exists := s.cfg.Projects[root]
	if body.Create {
		if exists {
			writeError(w, http.StatusConflict, "project is already declared: "+body.Root)
			return
		}
		if info, err := os.Stat(config.ExpandPath(root)); err != nil || !info.IsDir() {
			writeError(w, http.StatusBadRequest, "folder not found: "+body.Root)
			return
		}
	} else if !exists {
		writeError(w, http.StatusNotFound, "project not found: "+body.Root)
		return
	}

	projects := make(map[string]config.ManagedProject, len(s.cfg.Projects)+1)
	for k, v := range s.cfg.Projects {
		projects[k] = v
	}
	projects[root] = config.ManagedProject{Name: body.Name, Targets: body.Targets, Skills: body.Skills.config(), Agents: body.Agents.config()}
	if err := s.cfg.ValidateProjects(projects); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.cfg.Projects = projects
	if err := s.saveAndReloadConfig(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	action := "update"
	if body.Create {
		action = "add"
	}
	s.writeOpsLog("project", "ok", start, map[string]any{"action": action, "root": root, "scope": "ui"}, "")
	writeJSON(w, map[string]any{"success": true, "root": root})
}

func (s *Server) handleRemoveProject(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	root := s.declaredProjectRoot(r.URL.Query().Get("root"))
	if _, exists := s.cfg.Projects[root]; !exists {
		writeError(w, http.StatusNotFound, "project not found: "+root)
		return
	}
	abs := config.ExpandPath(root)
	for name, target := range s.cfg.Targets {
		if target.ProjectRoot() != abs {
			continue
		}
		if status, err := s.detachSkillsTarget(target.SkillsConfig().Path); err != nil {
			writeError(w, status, name+": "+err.Error())
			return
		}
	}
	delete(s.cfg.Projects, root)
	if err := s.saveAndReloadConfig(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeOpsLog("project", "ok", start, map[string]any{"action": "remove", "root": root, "scope": "ui"}, "")
	writeJSON(w, map[string]any{"success": true, "root": root})
}

// handleConvertProject moves the targets that write into one folder under projects.
// They keep their folders, mode and filters, so the next sync changes nothing.
func (s *Server) handleConvertProject(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var body struct {
		Root string `json:"root"`
	}
	if err := decodeJSON(w, r, &body, defaultJSONBodyLimit); err != nil {
		if !errors.Is(err, errBodyTooLarge) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, found := range s.cfg.ConvertibleProjects() {
		if found.Root != body.Root {
			continue
		}
		projects := make(map[string]config.ManagedProject, len(s.cfg.Projects)+1)
		for k, v := range s.cfg.Projects {
			projects[k] = v
		}
		projects[found.Root] = found.Project
		if err := s.cfg.ValidateProjects(projects); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		for _, name := range found.Targets {
			delete(s.cfg.Targets, name)
		}
		s.cfg.Projects = projects
		if err := s.saveAndReloadConfig(); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeOpsLog("project", "ok", start, map[string]any{"action": "convert", "root": found.Root, "targets": found.Targets, "scope": "ui"}, "")
		writeJSON(w, map[string]any{"success": true, "root": found.Root})
		return
	}
	writeError(w, http.StatusNotFound, "no targets to convert for "+body.Root)
}

// declaredProjectRoot returns the key a root is declared under, so ~/app and its
// absolute form name the same project. An undeclared root is returned as given.
func (s *Server) declaredProjectRoot(root string) string {
	abs := filepath.Clean(config.ExpandPath(root))
	for key := range s.cfg.Projects {
		if filepath.Clean(config.ExpandPath(key)) == abs {
			return key
		}
	}
	return root
}
