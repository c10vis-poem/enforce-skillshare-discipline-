package sourcewalk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"skillshare/internal/utils"
)

// Follow is one operation's policy for following directory links directly
// under the source root, one hop. It records each link it does not follow, so
// callers can warn once per operation and hold back deletions that would
// treat an unavailable link's skills as removed.
type Follow struct {
	root    string   // canonical source root
	targets []string // canonical active skills target paths

	mu      sync.Mutex
	skipped []Skipped
}

// Skipped is a first-level source link the policy did not follow.
type Skipped struct {
	Name   string // link name under the source root
	Reason string
	// Unavailable means the link's directory could not be read, so the
	// operation's inventory is incomplete.
	Unavailable bool
}

func (s Skipped) String() string {
	return fmt.Sprintf("source link %s not followed: %s", s.Name, s.Reason)
}

// NewFollow returns a policy for the given skills source and the paths of the
// active skills sync targets. Links resolving into, onto, or around a target
// are not followed.
func NewFollow(source string, targets []string) *Follow {
	f := &Follow{root: utils.ResolveSymlink(source)}
	for _, t := range targets {
		f.targets = append(f.targets, canonicalPath(t))
	}
	return f
}

// Skipped returns the links this policy declined so far, one per link.
func (f *Follow) Skipped() []Skipped {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Skipped(nil), f.skipped...)
}

// Incomplete reports whether a link could not be read, so the walked
// inventory is missing whatever that link holds.
func (f *Follow) Incomplete() bool {
	for _, s := range f.Skipped() {
		if s.Unavailable {
			return true
		}
	}
	return false
}

// Unavailable returns the names of links whose directory could not be read.
func (f *Follow) Unavailable() []string {
	var names []string
	for _, s := range f.Skipped() {
		if s.Unavailable {
			names = append(names, s.Name)
		}
	}
	return names
}

func (f *Follow) firstLevel(path string) bool {
	return utils.PathsEqual(filepath.Dir(path), f.root)
}

// Resolve maps a logical path whose first-level component under the source
// root is a followed link onto its real location, e.g. <source>/_dev-skills/foo
// to <checkout>/foo. ok is false when the path is not below a first-level link
// or the link is one this policy skips; callers then use path as is. The
// source root in path may be spelled through its own links.
func (f *Follow) Resolve(path string) (string, bool) {
	if f == nil {
		return "", false
	}
	link, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	var tail []string
	for {
		parent := filepath.Dir(link)
		if parent == link {
			return "", false
		}
		if utils.PathsEqual(utils.ResolveSymlink(parent), f.root) {
			break
		}
		tail = append([]string{filepath.Base(link)}, tail...)
		link = parent
	}
	info, err := os.Lstat(link)
	if err != nil || !utils.IsLinkMode(link, info.Mode()) {
		return "", false
	}
	target, _, ok := f.check(link)
	if !ok {
		return "", false
	}
	return filepath.Join(append([]string{target}, tail...)...), true
}

// check reports whether the link at path is followed, with its canonical
// target and a directory FileInfo carrying the link's own name.
func (f *Follow) check(path string) (string, os.FileInfo, bool) {
	name := filepath.Base(path)
	// Readlink first: on Windows, EvalSymlinks leaves a junction unresolved
	// (Go 1.23+ reports it as irregular, not a symlink), which would make the
	// walk below descend into the link entry itself and find nothing. The
	// link text is also readable while the target is away, so the where-it-
	// points guards run before the does-it-exist check: a link into a sync
	// target that is not created yet is an overlap, not an unavailable
	// inventory that would hold back pruning.
	target, err := utils.ResolveLinkTarget(path)
	if err != nil {
		return f.skip(name, unavailableReason(err), true)
	}
	target = canonicalPath(target)
	if within(f.root, target) {
		return f.skip(name, "target is the source or a parent of it", false)
	}
	for _, t := range f.targets {
		if within(t, target) || within(target, t) {
			return f.skip(name, "target overlaps sync target "+t, false)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return f.skip(name, unavailableReason(err), true)
	}
	if !info.IsDir() {
		// A link to a file (e.g. a shared .skillignore) is not a directory
		// link; it stays an ordinary entry.
		return "", nil, false
	}
	dir, err := os.Open(path)
	if err != nil {
		return f.skip(name, unavailableReason(err), true)
	}
	dir.Close()
	return target, namedInfo{info, name}, true
}

func (f *Follow) skip(name, reason string, unavailable bool) (string, os.FileInfo, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.skipped {
		if s.Name == name {
			return "", nil, false
		}
	}
	f.skipped = append(f.skipped, Skipped{Name: name, Reason: reason, Unavailable: unavailable})
	return "", nil, false
}

func unavailableReason(err error) string {
	switch {
	case os.IsNotExist(err):
		return "target is missing"
	case os.IsPermission(err):
		return "target is not readable"
	}
	return "target is unavailable: " + err.Error()
}

// within reports whether path is dir or below it, on separator boundaries.
func within(path, dir string) bool {
	return utils.PathsEqual(path, dir) || utils.PathHasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// canonicalPath resolves links in path. A missing tail is kept as written
// below its nearest existing parent, so a target not created yet still
// compares by where it will be.
func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	var missing []string
	for dir := abs; ; dir = filepath.Dir(dir) {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...)
		}
		if filepath.Dir(dir) == dir {
			return abs
		}
		missing = append([]string{filepath.Base(dir)}, missing...)
	}
}

// namedInfo reports a link target's FileInfo under the link's name.
type namedInfo struct {
	os.FileInfo
	name string
}

func (i namedInfo) Name() string { return i.name }
