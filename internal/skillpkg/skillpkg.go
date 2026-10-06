// Package skillpkg reads one skill directory as an Agent Skills package: its
// full frontmatter and every regular file, within the SEP-2640 limits. It has
// no configuration dependencies, so sync can share its naming rules.
package skillpkg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"skillshare/internal/sourcewalk"
	"skillshare/internal/utils"
)

// Per-skill limits from SEP-2640, and the Agent Skills field limits in characters.
const (
	MaxFiles         = 512
	MaxBytes         = 16 << 20
	MaxDescription   = 1024
	MaxCompatibility = 500
)

// File is one regular file of a package.
type File struct {
	Path string // absolute
	Rel  string // slash-separated, relative to the package directory
	Size int64
}

// Package is a valid skill directory.
type Package struct {
	Dir         string // resolved
	Frontmatter map[string]any
	SkillMD     []byte // the bytes Frontmatter was parsed from
	Files       []File
}

// Load reads the skill in dir. The error says why the directory is not a
// valid package. .git directories are left out; links are neither listed nor read.
func Load(dir string) (*Package, error) {
	dirName := filepath.Base(filepath.Clean(dir)) // a followed link's name, not its target's
	dir = utils.ResolveSymlink(dir)
	// A linked SKILL.md would be missing from the manifest, which lists no links.
	info, err := os.Lstat(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("SKILL.md is not a regular file")
	}
	// The read itself is bounded: the file can grow after the Lstat.
	content, err := readAtMost(filepath.Join(dir, "SKILL.md"), MaxBytes)
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
	desc, _ := fm["description"].(string)
	if strings.TrimSpace(desc) == "" {
		return nil, fmt.Errorf("SKILL.md is missing a description")
	}
	if n := utf8.RuneCountInString(desc); n > MaxDescription {
		return nil, fmt.Errorf("description has %d characters; the limit is %d", n, MaxDescription)
	}
	// metadata is left unchecked: the format wants string values, but lists there are
	// common and tool clients read them fine, so skipping those skills would cost more.
	compat, ok := fm["compatibility"].(string)
	if _, set := fm["compatibility"]; set && (!ok || compat == "") {
		return nil, fmt.Errorf("compatibility must be non-empty text")
	}
	if n := utf8.RuneCountInString(compat); n > MaxCompatibility {
		return nil, fmt.Errorf("compatibility has %d characters; the limit is %d", n, MaxCompatibility)
	}

	p := &Package{Dir: dir, Frontmatter: fm, SkillMD: content}
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
		p.Files = append(p.Files, File{Path: path, Rel: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// readAtMost reads the file, failing once it holds more than limit bytes.
func readAtMost(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("SKILL.md is over %d MiB", limit>>20)
	}
	return data, nil
}

// ValidateName returns why a SKILL.md name breaks the Agent Skills naming rules
// (including name == directory name), or "" when it follows them. Like the
// reference validator, it takes lowercase Unicode letters and digits after NFKC
// normalization and counts the limit in characters.
func ValidateName(name, dirName string) string {
	name = norm.NFKC.String(strings.TrimSpace(name))
	if name == "" {
		return "SKILL.md is missing a name"
	}
	if utf8.RuneCountInString(name) > 64 {
		return fmt.Sprintf("SKILL.md name %q is longer than 64 characters", name)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Sprintf("SKILL.md name %q cannot start or end with '-'", name)
	}
	if strings.Contains(name, "--") {
		return fmt.Sprintf("SKILL.md name %q cannot contain consecutive hyphens", name)
	}
	if name != strings.ToLower(name) || strings.ContainsFunc(name, func(r rune) bool {
		return r != '-' && !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		return fmt.Sprintf("SKILL.md name %q must use only lowercase letters, numbers, and hyphens", name)
	}
	if name != norm.NFKC.String(dirName) {
		return fmt.Sprintf("SKILL.md name %q does not match directory name %q", name, dirName)
	}
	return ""
}
