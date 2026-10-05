package server

import (
	"errors"
	"net/http"
	"time"

	"skillshare/internal/config"
	"skillshare/internal/sourcelink"
)

type sourceLinkRequest struct {
	Path   string `json:"path"`
	Name   string `json:"name,omitempty"`
	Enable bool   `json:"enable,omitempty"`
}

func (s *Server) handleCreateSourceLink(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var body sourceLinkRequest
	if err := decodeJSON(w, r, &body, defaultJSONBodyLimit); err != nil {
		if !errors.Is(err, errBodyTooLarge) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := sourcelink.Create(s.skillsSource(), config.SkillsTargetPaths(s.cfg.Targets), body.Path, body.Name)
	args := map[string]any{"name": body.Name, "target": body.Path, "enable": body.Enable, "scope": "ui"}
	if err != nil {
		s.writeOpsLog("link", "error", start, args, err.Error())
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer s.snapshotFollow()
	following := s.cfg.FollowSourceLinks
	if s.IsProjectMode() {
		following = s.projectCfg.FollowSourceLinks
	}
	if body.Enable && !following {
		if s.IsProjectMode() {
			s.projectCfg.FollowSourceLinks = true
		} else {
			s.cfg.FollowSourceLinks = true
		}
		if err := s.saveConfig(); err != nil {
			if s.IsProjectMode() {
				s.projectCfg.FollowSourceLinks = false
			} else {
				s.cfg.FollowSourceLinks = false
			}
			s.writeOpsLog("link", "error", start, args, err.Error())
			writeError(w, http.StatusInternalServerError, "link created, but failed to enable follow_source_links: "+err.Error())
			return
		}
	}
	s.writeOpsLog("link", "ok", start, args, "")
	writeJSON(w, map[string]any{"path": res.Path, "target": res.Target, "kind": res.Kind, "warning": res.Warning})
}

func (s *Server) handleRemoveSourceLink(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	name := r.PathValue("name")
	args := map[string]any{"name": name, "scope": "ui"}
	if err := sourcelink.Remove(s.skillsSource(), s.trashBase(), name); err != nil {
		s.writeOpsLog("unlink", "error", start, args, err.Error())
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.snapshotFollow()
	s.writeOpsLog("unlink", "ok", start, args, "")
	writeJSON(w, map[string]any{"success": true, "name": name})
}
