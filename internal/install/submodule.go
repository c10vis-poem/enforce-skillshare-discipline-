package install

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// gitlink is a submodule entry in a repo's tree. skillshare does not fetch
// submodules, so its directory is empty after any clone.
type gitlink struct {
	Path   string
	Commit string
	URL    string // from .gitmodules; "" when not recorded
}

func (l gitlink) String() string {
	s := fmt.Sprintf("'%s' (pinned at %s", l.Path, shortHash(l.Commit))
	if l.URL != "" {
		s += " from " + l.URL
	}
	return s + ")"
}

// repoGitlinks lists the submodules recorded in HEAD of repoPath. extraEnv
// authenticates the lazy .gitmodules fetch in a partial clone. Any git failure
// returns nil: callers only use the result to explain an empty directory.
func repoGitlinks(repoPath string, extraEnv []string) []gitlink {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	cmd := gitCommand(ctx, "ls-tree", "-r", "-z", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var links []gitlink
	for _, entry := range strings.Split(string(out), "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if ok && len(fields) == 3 && fields[0] == "160000" {
			links = append(links, gitlink{Path: path, Commit: fields[2]})
		}
	}
	if len(links) == 0 {
		return nil
	}

	cmd = gitCommand(ctx, "config", "-z", "--blob", "HEAD:.gitmodules", "--get-regexp", `^submodule\..*\.(path|url)$`)
	cmd.Dir = repoPath
	cmd.Env = append(cmd.Env, extraEnv...)
	out, err = cmd.Output()
	if err != nil {
		return links
	}
	paths, urls := map[string]string{}, map[string]string{}
	for _, entry := range strings.Split(string(out), "\x00") {
		key, val, _ := strings.Cut(entry, "\n")
		if name, ok := strings.CutSuffix(key, ".path"); ok {
			paths[name] = val
		} else if name, ok := strings.CutSuffix(key, ".url"); ok {
			urls[name] = val
		}
	}
	for name, path := range paths {
		for i := range links {
			if links[i].Path == path {
				links[i].URL = urls[name]
			}
		}
	}
	return links
}

// submoduleError refuses a subdir that is a submodule or lies inside one,
// which would otherwise install an empty directory or report a missing path.
func submoduleError(repoPath, subdir string, extraEnv []string) error {
	subdir = strings.Trim(filepath.ToSlash(subdir), "/")
	for _, l := range repoGitlinks(repoPath, extraEnv) {
		if subdir == l.Path || strings.HasPrefix(subdir, l.Path+"/") {
			return fmt.Errorf("'%s' is in git submodule %s, and skillshare does not fetch submodules; install from that repository instead, or copy the files into this one", subdir, l)
		}
	}
	return nil
}

// submoduleWarnings names each submodule whose contents a whole-repo install skips.
func submoduleWarnings(repoPath string, extraEnv []string) []string {
	var warnings []string
	for _, l := range repoGitlinks(repoPath, extraEnv) {
		warnings = append(warnings, fmt.Sprintf("skipped git submodule %s: skillshare does not fetch submodules", l))
	}
	return warnings
}
