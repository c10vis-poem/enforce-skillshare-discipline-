package skillserve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"skillshare/internal/config"
	"skillshare/internal/skillpkg"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillMD(name string) string {
	return "---\nname: " + name + "\ndescription: Use when testing " + name + "\nmetadata:\n  tags: [a, b]\n---\n# " + name + "\n"
}

func find(c *Catalog, uri string) *Skill {
	for _, s := range c.Skills {
		if s.URI == uri {
			return s
		}
	}
	return nil
}

func hasWarning(c *Catalog, substr string) bool {
	for _, w := range c.Skipped {
		if strings.Contains(w.String(), substr) {
			return true
		}
	}
	return false
}

func TestBuild_PublishesFrontmatterAndCompleteManifest(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "_team/tools/pdf/SKILL.md"), skillMD("pdf"))
	writeFile(t, filepath.Join(src, "_team/tools/pdf/references/FORMS.md"), "forms")
	writeFile(t, filepath.Join(src, "_team/tools/pdf/.git/HEAD"), "ref")

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	s := find(c, "skill://_team/tools/pdf/SKILL.md")
	if s == nil {
		t.Fatalf("skill not published; skills=%v warnings=%v", c.Skills, c.Skipped)
	}
	if s.Frontmatter["name"] != "pdf" || s.Frontmatter["metadata"] == nil {
		t.Errorf("frontmatter = %v, want full frontmatter", s.Frontmatter)
	}
	sum := sha256.Sum256([]byte("forms"))
	want := map[string]File{
		"skill://_team/tools/pdf/SKILL.md":            {Size: int64(len(skillMD("pdf")))},
		"skill://_team/tools/pdf/references/FORMS.md": {Size: 5, Digest: "sha256:" + hex.EncodeToString(sum[:])},
	}
	if len(s.Resources) != len(want) {
		t.Fatalf("resources = %+v, want SKILL.md and FORMS.md only", s.Resources)
	}
	for _, f := range s.Resources {
		w, ok := want[f.URI]
		if !ok || f.Size != w.Size || (w.Digest != "" && f.Digest != w.Digest) {
			t.Errorf("unexpected resource %+v", f)
		}
	}
}

func TestBuild_SkipsSkillWhoseNameDiffersFromDirectory(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "renamed/SKILL.md"), skillMD("original"))

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 0 || !hasWarning(c, `does not match directory name "renamed"`) {
		t.Errorf("skills=%v warnings=%v, want renamed skipped with a warning", c.Skills, c.Skipped)
	}
}

func TestBuild_AppliesTargetSelectionAndDisabledState(t *testing.T) {
	src := t.TempDir()
	for _, n := range []string{"keep", "drop", "off"} {
		writeFile(t, filepath.Join(src, n, "SKILL.md"), skillMD(n))
	}
	writeFile(t, filepath.Join(src, ".skillignore"), "off\n")

	c, err := (&Builder{Source: src, Target: "claude", Skills: &config.ResourceTargetConfig{Exclude: []string{"drop"}}}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 1 || c.Skills[0].URI != "skill://keep/SKILL.md" {
		t.Errorf("skills = %v, want only keep", c.Skills)
	}
}

func TestBuild_SkipsParentOfExcludedNestedSkill(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "suite/SKILL.md"), skillMD("suite"))
	writeFile(t, filepath.Join(src, "suite/child/SKILL.md"), skillMD("child"))

	c, err := (&Builder{Source: src, Target: "claude", Skills: &config.ResourceTargetConfig{Exclude: []string{"suite__child"}}}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 0 || !hasWarning(c, "suite/child") {
		t.Errorf("skills=%v warnings=%v, want suite skipped because of excluded child", c.Skills, c.Skipped)
	}
}

func TestBuild_SkipsParentOfInvalidNestedSkill(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "suite/SKILL.md"), skillMD("suite"))
	writeFile(t, filepath.Join(src, "suite/child/SKILL.md"), skillMD("other"))

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 0 || !hasWarning(c, "contains suite/child") {
		t.Errorf("skills=%v warnings=%v, want suite skipped because its child is skipped", c.Skills, c.Skipped)
	}
}

// A non-string key decodes to a map JSON cannot encode, which would fail the whole skills/list page.
func TestBuild_SkipsSkillWhoseFrontmatterJSONCannotCarry(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "ok/SKILL.md"), skillMD("ok"))
	writeFile(t, filepath.Join(src, "odd/SKILL.md"), "---\nname: odd\ndescription: Use when testing odd\nmetadata:\n  1: x\n---\n")

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 1 || c.Skills[0].URI != "skill://ok/SKILL.md" || !hasWarning(c, "skipped odd") {
		t.Errorf("skills=%v warnings=%v, want odd skipped and ok served", c.Skills, c.Skipped)
	}
}

// The Skills extension requires SKILL.md to begin with its frontmatter.
func TestBuild_SkipsSkillWhoseFrontmatterIsNotAtTheStart(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "late/SKILL.md"), "# intro\n"+skillMD("late"))
	writeFile(t, filepath.Join(src, "open/SKILL.md"), "---\nname: open\ndescription: Use when testing open\n# open\n")

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 0 || !hasWarning(c, "skipped late") || !hasWarning(c, "skipped open") {
		t.Errorf("skills=%v warnings=%v, want late and unclosed open skipped", c.Skills, c.Skipped)
	}
}

// The Agent Skills format caps a description at 1,024 characters, not bytes.
func TestBuild_SkipsSkillWhoseDescriptionIsTooLong(t *testing.T) {
	src := t.TempDir()
	skill := func(name, desc string) string { return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" }
	writeFile(t, filepath.Join(src, "long/SKILL.md"), skill("long", strings.Repeat("a", 1025)))
	writeFile(t, filepath.Join(src, "wide/SKILL.md"), skill("wide", strings.Repeat("技", 1024)))

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 1 || c.Skills[0].URI != "skill://wide/SKILL.md" || !hasWarning(c, "skipped long") {
		t.Errorf("skills=%v warnings=%v, want long skipped and wide served", c.Skills, c.Skipped)
	}
}

func TestBuild_SkipsSkillOverFileLimit(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "big/SKILL.md"), skillMD("big"))
	for i := range skillpkg.MaxFiles {
		writeFile(t, filepath.Join(src, "big/refs", fmt.Sprintf("f%03d", i)), "")
	}

	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Skills) != 0 || !hasWarning(c, "512") {
		t.Errorf("skills=%d warnings=%v, want big skipped over the file limit", len(c.Skills), c.Skipped)
	}
}

func TestBuild_RefreshesDigestWhenContentChanges(t *testing.T) {
	src := t.TempDir()
	ref := filepath.Join(src, "doc/notes.md")
	writeFile(t, filepath.Join(src, "doc/SKILL.md"), skillMD("doc"))
	writeFile(t, ref, "one")
	b := &Builder{Source: src}
	first, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	// Same size and modification time, as cp -p or rsync -t leave it.
	info, err := os.Stat(ref)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, ref, "two")
	if err := os.Chtimes(ref, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	second, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	digest := func(c *Catalog) string {
		for _, f := range c.Skills[0].Resources {
			if strings.HasSuffix(f.URI, "notes.md") {
				return f.Digest
			}
		}
		return ""
	}
	if digest(first) == digest(second) {
		t.Error("digest did not change after the file changed")
	}
}

func TestRead_ServesOnlyManifestFilesInsideTheSkill(t *testing.T) {
	src := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "secret")
	writeFile(t, filepath.Join(src, "doc/SKILL.md"), skillMD("doc"))
	if err := os.Symlink(outside, filepath.Join(src, "doc/leak.txt")); err != nil {
		t.Fatal(err)
	}
	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}

	if data, err := c.Read("skill://doc/SKILL.md"); err != nil || string(data) != skillMD("doc") {
		t.Errorf("Read SKILL.md = %q, %v", data, err)
	}
	for _, uri := range []string{"skill://doc/leak.txt", "skill://doc/../doc/SKILL.md", "skill://doc/%2e%2e/doc/SKILL.md"} {
		if _, err := c.Read(uri); err == nil {
			t.Errorf("Read(%s) succeeded, want rejection", uri)
		}
	}
}

func TestRead_ServesListedFileWithBackslashInName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a backslash separates paths on Windows")
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "doc/SKILL.md"), skillMD("doc"))
	writeFile(t, filepath.Join(src, `doc/a\b.txt`), "x")
	c, err := (&Builder{Source: src}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if data, err := c.Read("skill://doc/a%5Cb.txt"); err != nil || string(data) != "x" {
		t.Errorf("Read = %q, %v; want the listed file", data, err)
	}
}
