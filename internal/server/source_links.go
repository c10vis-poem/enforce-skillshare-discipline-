package server

import (
	"fmt"
	"path/filepath"

	"skillshare/internal/install"
	"skillshare/internal/sourcewalk"
)

type linkedRepoInfo struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

// dashboardRepos separates user-owned checkouts before any Git checks or updates.
func dashboardRepos(source string, walk sourcewalk.Options) ([]string, []linkedRepoInfo) {
	repos, _ := install.GetTrackedRepos(source, walk)
	managed := make([]string, 0, len(repos))
	linked := []linkedRepoInfo{}
	seen := map[string]bool{}
	for _, rel := range repos {
		// The dashboard matches repos by slash path (org/_team); GetTrackedRepos is OS-native.
		path, name := filepath.Join(source, rel), filepath.ToSlash(rel)
		if target, ok := walk.Follow.Resolve(path); ok {
			linked = append(linked, linkedRepoInfo{Name: name, Target: target})
			seen[name] = true
		} else {
			managed = append(managed, name)
		}
	}
	// A followed checkout need not have the tracked-repo underscore prefix.
	entries, _ := sourcewalk.ReadDir(source, walk)
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(source, name)
		if target, ok := walk.Follow.Resolve(path); ok && !seen[name] && install.IsGitRepo(path) {
			linked = append(linked, linkedRepoInfo{Name: name, Target: target})
		}
	}
	return managed, linked
}

// followedCheckout also protects skills inside a followed Git checkout.
func followedCheckout(path string, follow *sourcewalk.Follow) string {
	for {
		target, ok := follow.Resolve(path)
		if !ok {
			return ""
		}
		if install.IsGitRepo(path) {
			return target
		}
		path = filepath.Dir(path)
	}
}

func (s *Server) refuseFollowedCheckout(name, path string, follow []*sourcewalk.Follow) *updateResultItem {
	var policy *sourcewalk.Follow
	if len(follow) > 0 {
		policy = follow[0]
	} else {
		policy = s.skillsWalk().Follow
	}
	if target := followedCheckout(path, policy); target != "" {
		return &updateResultItem{Name: name, Action: "skipped", IsRepo: true,
			Message: fmt.Sprintf("followed checkout %s is managed by you and is not updated by the dashboard", target)}
	}
	return nil
}
