// Package skillserve publishes the skills source as an MCP Skills extension
// (SEP-2640) catalog: one entry per valid skill with its full frontmatter and a
// complete file manifest, and contained reads of the listed files.
package skillserve

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"

	"skillshare/internal/config"
	"skillshare/internal/skillpkg"
	"skillshare/internal/sourcewalk"
	ssync "skillshare/internal/sync"
)

// extensionID is the SEP-2640 capability key.
const extensionID = "io.modelcontextprotocol/skills"

var errNotFound = errors.New("not found")

// File is one manifest entry of a skill.
type File struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// Skill is one skills/list entry.
type Skill struct {
	URI         string         `json:"uri"`
	Frontmatter map[string]any `json:"frontmatter"`
	Resources   []File         `json:"resources"`
}

// servedFile locates a manifest file: the resolved skill directory, the
// slash path inside it, and the digest it was listed with.
type servedFile struct{ dir, rel, digest string }

// Skip is a skill left out of the catalog, by source path.
type Skip struct {
	Path, Reason string
}

func (s Skip) String() string { return fmt.Sprintf("skipped %s: %s", s.Path, s.Reason) }

// Catalog is one snapshot of the served skills, ordered by source path.
type Catalog struct {
	Skills  []*Skill
	Skipped []Skip
	bySkill map[string]*Skill
	byFile  map[string]servedFile
}

// Builder builds catalogs from the skills source.
type Builder struct {
	Source string
	Walk   sourcewalk.Options
	// Target names the target whose Skills selection applies. With Skills nil,
	// every enabled skill is served.
	Target string
	Skills *config.ResourceTargetConfig
}

// Build discovers the source and returns the skills to serve. Skills that are
// invalid, over the limits, or contain an unserved nested skill are skipped
// with a warning.
func (b *Builder) Build() (*Catalog, error) {
	all, err := ssync.DiscoverSourceSkillsAll(b.Source, b.Walk)
	if err != nil {
		return nil, err
	}
	var selected []ssync.DiscoveredSkill
	if b.Skills != nil {
		if selected, err = ssync.SelectTargetSkills(all, b.Target, *b.Skills); err != nil {
			return nil, err
		}
	} else {
		selected = slices.DeleteFunc(slices.Clone(all), func(s ssync.DiscoveredSkill) bool { return s.Disabled })
	}
	served := make(map[string]bool, len(selected))
	for _, s := range selected {
		served[s.RelPath] = true
	}
	var unserved []string
	for _, s := range all {
		if !served[s.RelPath] {
			unserved = append(unserved, s.RelPath)
		}
	}
	slices.SortFunc(selected, func(x, y ssync.DiscoveredSkill) int { return strings.Compare(x.RelPath, y.RelPath) })

	type loaded struct {
		skill  *Skill
		files  map[string]servedFile
		reason string
	}
	loads := make([]loaded, len(selected))
	for i, s := range selected {
		loads[i].skill, loads[i].files, loads[i].reason = load(s)
	}
	// Deepest first, so a nested skill skipped for any reason also skips its parents,
	// which would otherwise publish its files as their own.
	for i := len(loads) - 1; i >= 0; i-- {
		for _, o := range unserved {
			if loads[i].reason == "" && strings.HasPrefix(o, selected[i].RelPath+"/") {
				loads[i].reason = fmt.Sprintf("contains %s, which is not served", o)
			}
		}
		if loads[i].reason != "" {
			unserved = append(unserved, selected[i].RelPath)
		}
	}

	c := &Catalog{bySkill: map[string]*Skill{}, byFile: map[string]servedFile{}}
	for i, l := range loads {
		if l.reason != "" {
			c.Skipped = append(c.Skipped, Skip{Path: selected[i].RelPath, Reason: l.reason})
			continue
		}
		c.Skills = append(c.Skills, l.skill)
		c.bySkill[l.skill.URI] = l.skill
		maps.Copy(c.byFile, l.files)
	}
	return c, nil
}

// load builds one entry and its files, or returns why the skill is skipped.
// Every build hashes every file: size and mtime cannot prove the content is
// unchanged (cp -p, rsync -t), and a stale digest makes hosts reject the file.
func load(s ssync.DiscoveredSkill) (*Skill, map[string]servedFile, string) {
	p, err := skillpkg.Load(s.SourcePath)
	if err != nil {
		return nil, nil, err.Error()
	}
	base := "skill://" + escapePath(s.RelPath)
	skill := &Skill{URI: base + "/SKILL.md", Frontmatter: p.Frontmatter}
	files := make(map[string]servedFile, len(p.Files))
	for _, f := range p.Files {
		sum, size, err := hashFile(f.Path)
		if err != nil {
			return nil, nil, err.Error()
		}
		uri := base + "/" + escapePath(f.Rel)
		files[uri] = servedFile{dir: p.Dir, rel: f.Rel, digest: sum}
		skill.Resources = append(skill.Resources, File{URI: uri, Digest: sum, Size: size})
	}
	return skill, files, ""
}

// hashFile returns the digest and size of one read of the file, so both describe
// the same bytes even when the file is replaced while the catalog is built.
func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), n, nil
}

// Skill returns the served skill whose SKILL.md URI is uri.
func (c *Catalog) Skill(uri string) (*Skill, bool) {
	key, ok := canonical(uri)
	skill := c.bySkill[key]
	return skill, ok && skill != nil
}

// Read returns the content of a listed file. Only manifest files are served,
// and the read is confined to the skill directory.
func (c *Catalog) Read(uri string) ([]byte, error) {
	key, ok := canonical(uri)
	sf, listed := c.byFile[key]
	if !ok || !listed {
		return nil, errNotFound
	}
	root, err := os.OpenRoot(sf.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(sf.rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, skillpkg.MaxBytes))
	if err != nil {
		return nil, err
	}
	// A file edited since it was listed would fail the client's digest check;
	// the next refresh lists it again.
	if fmt.Sprintf("sha256:%x", sha256.Sum256(data)) != sf.digest {
		return nil, fmt.Errorf("%s changed since it was listed; list the skills again", uri)
	}
	return data, nil
}

func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// canonical decodes a skill:// URI and re-encodes it the way the catalog
// builds URIs. Empty, dot, and separator-bearing segments are rejected.
func canonical(uri string) (string, bool) {
	rest, ok := strings.CutPrefix(uri, "skill://")
	if !ok {
		return "", false
	}
	segs := strings.Split(rest, "/")
	for i, s := range segs {
		d, err := url.PathUnescape(s)
		if err != nil || d == "" || d == "." || d == ".." || strings.ContainsAny(d, "/\x00") {
			return "", false
		}
		segs[i] = url.PathEscape(d)
	}
	return "skill://" + strings.Join(segs, "/"), true
}
