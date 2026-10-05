package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceLinkWarnings(t *testing.T) {
	for _, subdir := range []string{"", "skills"} {
		for _, state := range []string{"untracked", "ignored", "indexed", "indexed-ignored", "directory"} {
			t.Run(subdir+"/"+state, func(t *testing.T) {
				root := initTestRepo(t)
				skills := filepath.Join(root, subdir)
				if err := os.MkdirAll(skills, 0755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(skills, "_dev-skills")
				if state == "directory" {
					if err := os.Mkdir(path, 0755); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Symlink(t.TempDir(), path); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(state, "indexed") {
					cmd := exec.Command("git", "add", "--", path)
					cmd.Dir = root
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("add: %v %s", err, out)
					}
				}
				pattern := "/" + filepath.ToSlash(filepath.Join(subdir, "_dev-skills"))
				if strings.Contains(state, "ignored") {
					if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(pattern+"\n"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				warnings := SourceLinkWarnings(root, skills, "commit")
				if state == "ignored" || state == "directory" {
					if len(warnings) != 0 {
						t.Fatalf("unexpected warnings: %v", warnings)
					}
					return
				}
				if len(warnings) != 1 {
					t.Fatalf("warnings = %v", warnings)
				}
				for _, want := range []string{pattern, "commit will stage source link", filepath.Join(root, ".gitignore")} {
					if !strings.Contains(warnings[0], want) {
						t.Errorf("missing %q in %q", want, warnings[0])
					}
				}
				if strings.HasPrefix(state, "indexed") && !strings.Contains(warnings[0], "rm --cached") {
					t.Errorf("missing untrack instruction: %s", warnings[0])
				}
				var stagedWarnings []string
				if err := StageAll(root, skills, "commit", func(w string) { stagedWarnings = append(stagedWarnings, w) }); err != nil {
					t.Fatal(err)
				}
				if len(stagedWarnings) != 1 {
					t.Fatalf("staging warnings = %v", stagedWarnings)
				}
				cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", path)
				cmd.Dir = root
				if err := cmd.Run(); err != nil {
					t.Fatal("warning must not prevent staging:", err)
				}
			})
		}
	}
}

func TestSourceLinkWarnings_LiteralPathsAndFirstLevelOnly(t *testing.T) {
	root := initTestRepo(t)
	name := "_dev [a]*' skills"
	if err := os.Symlink(t.TempDir(), filepath.Join(root, name)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "group"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "group", "nested")); err != nil {
		t.Fatal(err)
	}
	warnings := SourceLinkWarnings(root, root, "push")
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
	// Apply the exact suggested pattern, then ask Git to prove it ignores the link.
	pattern := strings.Split(warnings[0], "`")[1]
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(pattern+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := SourceLinkWarnings(root, root, "push"); len(got) != 0 {
		t.Fatalf("pattern did not ignore literal link: %v", got)
	}
	cmd := exec.Command("git", "--literal-pathspecs", "add", "-f", "--", name)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add: %v %s", err, out)
	}
	warnings = SourceLinkWarnings(root, root, "push")
	command := strings.Split(warnings[0], "run from the git root: ")[1]
	cmd = exec.Command("sh", "-c", command)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("manual untrack: %v %s", err, out)
	}
	if got := SourceLinkWarnings(root, root, "push"); len(got) != 0 {
		t.Fatalf("manual instruction did not untrack literal link: %v", got)
	}
	if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
		t.Fatal("untrack removed link:", err)
	}
}
