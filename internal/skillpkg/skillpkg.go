// Package skillpkg reads one skill directory as an Agent Skills package: its
// full frontmatter and every regular file, within the SEP-2640 limits. It has
// no configuration dependencies, so sync can share its naming rules.
package skillpkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"skillshare/internal/sourcewalk"
	"skillshare/internal/utils"
)

// Per-skill limits from SEP-2640.
const (
	MaxFiles = 512
	MaxBytes = 16 << 20
)

// File is one regular file of a package.
type File struct {
	Path    string // absolute
	Rel     string // slash-separated, relative to the package directory
	Size    int64
	ModTime time.Time
}

// Package is a valid skill directory.
type Package struct {
	Dir         string // resolved
	Frontmatter map[string]any
	Files       []File
}

// Load reads the skill in dir. The error says why the directory is not a
// valid package. .git directories are left out; links are neither listed nor read.
func Load(dir string) (*Package, error) {
	dirName := filepath.Base(filepath.Clean(dir)) // a followed link's name, not its target's
	dir = utils.ResolveSymlink(dir)
	content, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	fm, err := utils.ParseFrontmatterMap(content)
	if err != nil {
		return nil, fmt.Errorf("SKILL.md has %w", err)
	}
	name, _ := fm["name"].(string)
	if reason := ValidateName(name, dirName); reason != "" {
		return nil, fmt.Errorf("%s", reason)
	}
	if desc, _ := fm["description"].(string); strings.TrimSpace(desc) == "" {
		return nil, fmt.Errorf("SKILL.md is missing a description")
	}

	p := &Package{Dir: dir, Frontmatter: fm}
	var total int64
	err = sourcewalk.Walk(dir, sourcewalk.Options{}, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if len(p.Files) == MaxFiles {
			return fmt.Errorf("has more than %d files", MaxFiles)
		}
		if total += info.Size(); total > MaxBytes {
			return fmt.Errorf("is larger than 16 MiB")
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		p.Files = append(p.Files, File{Path: path, Rel: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ValidateName returns why a SKILL.md name breaks the Agent Skills naming rules
// (including name == directory name), or "" when it follows them.
func ValidateName(name, dirName string) string {
	if name == "" {
		return "SKILL.md is missing a name"
	}
	if len(name) > 64 {
		return fmt.Sprintf("SKILL.md name %q is longer than 64 characters", name)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Sprintf("SKILL.md name %q cannot start or end with '-'", name)
	}
	if strings.Contains(name, "--") {
		return fmt.Sprintf("SKILL.md name %q cannot contain consecutive hyphens", name)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Sprintf("SKILL.md name %q must use only lowercase letters, numbers, and hyphens", name)
	}
	if name != dirName {
		return fmt.Sprintf("SKILL.md name %q does not match directory name %q", name, dirName)
	}
	return ""
}
