// Package skillserve publishes the skills source as an MCP Skills extension
// (SEP-2640) catalog: one entry per valid skill with its full frontmatter and a
// complete file manifest, and contained reads of the listed files.
package skillserve

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"skillshare/internal/config"
	"skillshare/internal/skillpkg"
	"skillshare/internal/sourcewalk"
	ssync "skillshare/internal/sync"
	"skillshare/internal/utils"
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

// servedFile locates a manifest file: the resolved skill directory and the
// slash path inside it.
type servedFile struct{ dir, rel string }

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

// Builder builds catalogs from the skills source. It keeps the digests of the
// last build so unchanged files are not hashed again.
type Builder struct {
	Source string
	Walk   sourcewalk.Options
	// Target names the target whose Skills selection applies. With Skills nil,
	// every enabled skill is served.
	Target string
	Skills *config.ResourceTargetConfig

	digests map[string]digest
}

type digest struct {
	size  int64
	mtime time.Time
	sum   string
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

	c := &Catalog{bySkill: map[string]*Skill{}, byFile: map[string]servedFile{}}
	next := map[string]digest{}
	for _, s := range selected {
		skill, reason := b.load(s, unserved, c.byFile, next)
		if reason != "" {
			c.Skipped = append(c.Skipped, Skip{Path: s.RelPath, Reason: reason})
			continue
		}
		c.Skills = append(c.Skills, skill)
		c.bySkill[skill.URI] = skill
	}
	b.digests = next
	return c, nil
}

// load builds one entry and registers its files in byFile, or returns why the
// skill is skipped.
func (b *Builder) load(s ssync.DiscoveredSkill, unserved []string, byFile map[string]servedFile, next map[string]digest) (*Skill, string) {
	for _, o := range unserved {
		if strings.HasPrefix(o, s.RelPath+"/") {
			return nil, fmt.Sprintf("contains %s, which is not served", o)
		}
	}
	p, err := skillpkg.Load(s.SourcePath)
	if err != nil {
		return nil, err.Error()
	}
	base := "skill://" + escapePath(s.RelPath)
	skill := &Skill{URI: base + "/SKILL.md", Frontmatter: p.Frontmatter}
	files := make(map[string]servedFile, len(p.Files))
	for _, f := range p.Files {
		sum, err := b.digest(f, next)
		if err != nil {
			return nil, err.Error()
		}
		uri := base + "/" + escapePath(f.Rel)
		files[uri] = servedFile{dir: p.Dir, rel: f.Rel}
		skill.Resources = append(skill.Resources, File{URI: uri, Digest: sum, Size: f.Size})
	}
	for uri, f := range files {
		byFile[uri] = f
	}
	return skill, ""
}

// ponytail: size+mtime keys the digest cache, so a same-size edit within the
// filesystem's mtime granularity keeps the old digest until the next change.
func (b *Builder) digest(f skillpkg.File, next map[string]digest) (string, error) {
	if d, ok := b.digests[f.Path]; ok && d.size == f.Size && d.mtime.Equal(f.ModTime) {
		next[f.Path] = d
		return d.sum, nil
	}
	sum, err := utils.FileHashFormatted(f.Path)
	if err != nil {
		return "", err
	}
	next[f.Path] = digest{size: f.Size, mtime: f.ModTime, sum: sum}
	return sum, nil
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
	return io.ReadAll(io.LimitReader(f, skillpkg.MaxBytes))
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
		if err != nil || d == "" || d == "." || d == ".." || strings.ContainsAny(d, "/\\\x00") {
			return "", false
		}
		segs[i] = url.PathEscape(d)
	}
	return "skill://" + strings.Join(segs, "/"), true
}
