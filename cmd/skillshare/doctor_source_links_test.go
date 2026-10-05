package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/sourcewalk"
)

func TestCheckUndeclaredSourceLinks_FirstLevelOnlyAndSymlinkedRoot(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	skill := filepath.Join(source, "real-skill")
	if err := os.MkdirAll(skill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Real skill"), 0644); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "file.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		".hidden-link": external,
		"broken-link":  filepath.Join(base, "missing"),
		"dir-link":     external,
		"file-link":    filepath.Join(external, "file.md"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(source, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(external, filepath.Join(skill, "nested-link")); err != nil {
		t.Fatal(err)
	}
	rootLink := filepath.Join(base, "source-link")
	if err := os.Symlink(source, rootLink); err != nil {
		t.Fatal(err)
	}

	result := &doctorResult{}
	output := captureStdout(t, func() { checkUndeclaredSourceLinks(rootLink, sourcewalk.Options{}, result) })
	if result.errors != 0 || result.warnings != 0 {
		t.Errorf("source links must be informational: %+v", result)
	}
	if len(result.checks) != len(links) {
		t.Fatalf("got %d checks, want %d: %+v", len(result.checks), len(links), result.checks)
	}
	for _, check := range result.checks {
		name := strings.SplitN(check.Message, ":", 2)[0]
		if _, ok := links[name]; !ok {
			t.Errorf("unexpected link reported: %+v", check)
		}
		delete(links, name)
		if check.Name != "undeclared_source_links" || check.Status != checkInfo {
			t.Errorf("unexpected link check: %+v", check)
		}
		if !strings.Contains(output, check.Message) {
			t.Errorf("text output missing %q: %s", check.Message, output)
		}
	}
	if strings.Contains(output, "nested-link") || strings.Contains(output, ".skillfollow") {
		t.Errorf("unexpected source link output: %s", output)
	}
	for _, name := range []string{"dir-link", "broken-link"} {
		if !strings.Contains(output, name+": not followed by discovery; its contents are invisible to skillshare. Set follow_source_links: true to follow it") {
			t.Errorf("%s: missing follow_source_links hint: %s", name, output)
		}
	}
	jsonOutput := buildDoctorOutput(result)
	if jsonOutput.Summary.Info != 4 || jsonOutput.Summary.Warnings != 0 || jsonOutput.Summary.Errors != 0 {
		t.Errorf("unexpected JSON summary: %+v", jsonOutput.Summary)
	}
}

func TestCheckUndeclaredSourceLinks_NoLinksIsSilent(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "real-skill"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "real-skill", "SKILL.md"), []byte("# Real skill"), 0644); err != nil {
		t.Fatal(err)
	}
	result := &doctorResult{}
	output := captureStdout(t, func() { checkUndeclaredSourceLinks(source, sourcewalk.Options{}, result) })
	if output != "" || len(result.checks) != 0 || result.errors != 0 || result.warnings != 0 {
		t.Errorf("no-link source changed doctor output: %q, %+v", output, result)
	}
}

func TestCheckUndeclaredSourceLinks_FollowEnabled(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(base, "checkout")
	if err := os.MkdirAll(filepath.Join(checkout, "foo"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"_dev-skills": checkout,
		"gone":        filepath.Join(base, "missing"),
		"loop":        base,
	} {
		if err := os.Symlink(target, filepath.Join(source, name)); err != nil {
			t.Fatal(err)
		}
	}

	result := &doctorResult{}
	walk := sourcewalk.Options{Follow: sourcewalk.NewFollow(source, nil)}
	output := captureStdout(t, func() { checkUndeclaredSourceLinks(source, walk, result) })
	for _, want := range []string{
		"_dev-skills: followed as a directory (follow_source_links)",
		"gone: not followed: target is missing",
		"loop: not followed: target is the source or a parent of it",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "invisible") {
		t.Errorf("enabled follow must not claim links are invisible: %s", output)
	}
	if result.warnings != 2 {
		t.Errorf("want a warning per skipped link, got %+v", result)
	}
}
