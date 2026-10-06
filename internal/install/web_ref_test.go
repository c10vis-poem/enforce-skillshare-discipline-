package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRefsRemote makes a repo with a "feature/x" branch and a "v1.0" tag.
func newRefsRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "r") // matches the repo name in the test URLs
	runGit(t, "", "init", "-q", "-b", "main", dir)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-q", "-m", "init")
	runGit(t, dir, "branch", "feature/x")
	runGit(t, dir, "tag", "v1.0")
	return dir
}

func parseWithRemote(t *testing.T, raw, remote string) *Source {
	t.Helper()
	source, err := ParseSource(raw)
	if err != nil {
		t.Fatalf("ParseSource(%q): %v", raw, err)
	}
	source.CloneURL = fileURL(remote)
	return source
}

func TestResolveWebRef(t *testing.T) {
	remote := newRefsRemote(t)

	tests := []struct {
		name       string
		raw        string
		branch     string // set after parsing, as --branch or a saved config would
		repoRoot   bool   // Subdir cleared, as for a grouped whole-repo clone
		wantBranch string
		wantSubdir string
		wantName   string
		wantErr    string
	}{
		{
			name:       "single-segment tag is kept",
			raw:        "github.com/o/r/tree/v1.0/skills/foo",
			wantBranch: "v1.0",
			wantSubdir: "skills/foo",
		},
		{
			name:       "HEAD means the default branch",
			raw:        "github.com/o/r/tree/HEAD/skills/foo",
			wantBranch: "",
			wantSubdir: "skills/foo",
		},
		{
			name:       "branch containing a slash",
			raw:        "github.com/o/r/tree/feature/x/skills/foo",
			wantBranch: "feature/x",
			wantSubdir: "skills/foo",
		},
		{
			name:       "branch containing a slash with no subdir",
			raw:        "github.com/o/r/tree/feature/x",
			wantBranch: "feature/x",
			wantSubdir: "",
			wantName:   "r",
		},
		{
			name:       "saved slash branch that is the whole tail",
			raw:        "github.com/o/r/tree/feature/x",
			branch:     "feature/x",
			wantBranch: "feature/x",
			wantSubdir: "",
			wantName:   "r",
		},
		{
			name:       "unrelated --branch keeps the URL's real subdir",
			raw:        "github.com/o/r/tree/feature/x/skills/foo",
			branch:     "main",
			wantBranch: "main",
			wantSubdir: "skills/foo",
		},
		{
			name:       "repo-root copy resolves the ref but keeps its root",
			raw:        "github.com/o/r/tree/feature/x/skills/foo",
			repoRoot:   true,
			wantBranch: "feature/x",
			wantSubdir: "",
		},
		{
			name:       "gitlab branch containing a slash",
			raw:        "https://gitlab.com/o/r/-/tree/feature/x/skills/foo",
			wantBranch: "feature/x",
			wantSubdir: "skills/foo",
		},
		{
			name:       "saved branch moves the subdir without asking the remote",
			raw:        "github.com/o/r/tree/other/y/skills/foo",
			branch:     "other/y",
			wantBranch: "other/y",
			wantSubdir: "skills/foo",
		},
		{
			name:       "unrelated --branch wins",
			raw:        "github.com/o/r/tree/v1.0/skills/foo",
			branch:     "main",
			wantBranch: "main",
			wantSubdir: "skills/foo",
		},
		{
			name:    "unknown ref fails with a hint",
			raw:     "github.com/o/r/tree/v9.9/skills/foo",
			wantErr: "--branch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := parseWithRemote(t, tt.raw, remote)
			if tt.branch != "" {
				source.Branch = tt.branch
			}
			if tt.repoRoot {
				source.Subdir = ""
			}
			err := resolveWebRef(source)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveWebRef: %v", err)
			}
			if source.Branch != tt.wantBranch {
				t.Errorf("Branch = %q, want %q", source.Branch, tt.wantBranch)
			}
			if source.Subdir != tt.wantSubdir {
				t.Errorf("Subdir = %q, want %q", source.Subdir, tt.wantSubdir)
			}
			if tt.wantName != "" && source.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", source.Name, tt.wantName)
			}
		})
	}
}

func TestRemoteRefs_ListsEachRemoteOnce(t *testing.T) {
	remote := newRefsRemote(t)
	var refs RemoteRefs

	first := parseWithRemote(t, "github.com/o/r/tree/feature/x/skills/a", remote)
	if err := refs.Resolve(first); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// With the remote gone, only the cached refs can resolve the second URL.
	if err := os.RemoveAll(remote); err != nil {
		t.Fatal(err)
	}
	second := parseWithRemote(t, "github.com/o/r/tree/feature/x/skills/b", remote)
	if err := refs.Resolve(second); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if second.Branch != "feature/x" || second.Subdir != "skills/b" || second.HasAmbiguousWebRef() {
		t.Errorf("second = branch %q subdir %q ambiguous %v, want feature/x, skills/b, false",
			second.Branch, second.Subdir, second.HasAmbiguousWebRef())
	}
}

func TestApplyRecordedBranch(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		branch     string
		wantBranch string
		wantSubdir string
		ambiguous  bool
	}{
		{"legacy install drops the URL ref", "github.com/o/r/tree/v1.0/skills/foo", "", "", "skills/foo", false},
		{"recorded slash branch settles the subdir", "github.com/o/r/tree/feature/x/skills/foo", "feature/x", "feature/x", "skills/foo", false},
		{"recorded first segment settles the subdir", "github.com/o/r/tree/v1.0/skills/foo", "v1.0", "v1.0", "skills/foo", false},
		{"unrelated branch still needs the remote", "github.com/o/r/tree/feature/x/skills/foo", "main", "main", "x/skills/foo", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := ParseSource(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			source.ApplyRecordedBranch(tt.branch)
			if source.Branch != tt.wantBranch || source.Subdir != tt.wantSubdir || source.HasAmbiguousWebRef() != tt.ambiguous {
				t.Errorf("got branch %q subdir %q ambiguous %v, want %q %q %v",
					source.Branch, source.Subdir, source.HasAmbiguousWebRef(), tt.wantBranch, tt.wantSubdir, tt.ambiguous)
			}
		})
	}
}

func TestRemoteRefs_ListSplitsBranchesAndTags(t *testing.T) {
	remote := newRefsRemote(t)
	for _, tag := range []string{"v1.10.0", "v1.2.0", "v1.2.0-rc.1", "nightly"} {
		runGit(t, remote, "tag", "-a", tag, "-m", tag)
	}
	var refs RemoteRefs
	list, err := refs.List(parseWithRemote(t, "github.com/o/r", remote))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := fmt.Sprintf("%s %v %v", list.DefaultBranch, list.Branches, list.Tags)
	want := "main [feature/x main] [v1.10.0 v1.2.0 v1.2.0-rc.1 v1.0 nightly]"
	if got != want {
		t.Errorf("refs = %s, want %s", got, want)
	}
}

func TestSourceAtRef(t *testing.T) {
	tests := []struct {
		name, raw, ref, want string
	}{
		{"github shorthand", "o/r/skills/foo", "v1.2.0", "github.com/o/r/tree/v1.2.0/skills/foo"},
		{"github URL replaces its ref", "https://github.com/o/r/tree/v1.0/skills/foo", "main", "github.com/o/r/tree/main/skills/foo"},
		{"slash branch is replaced whole", "github.com/o/r/tree/feature/x/skills/foo", "v1.0", "github.com/o/r/tree/v1.0/skills/foo"},
		{"ref the remote lacks is replaced", "github.com/o/r/tree/master/skills/foo", "main", "github.com/o/r/tree/main/skills/foo"},
		{"github file URL stays a file URL", "github.com/o/r/blob/v1.0/skills/foo/SKILL.md", "v2", "github.com/o/r/blob/v2/skills/foo/SKILL.md"},
		{"github default branch drops the ref", "github.com/o/r/tree/v1.0/skills/foo", "", "github.com/o/r/skills/foo"},
		{"gitlab", "https://gitlab.com/g/r/-/tree/main/skills/foo", "v1.0", "https://gitlab.com/g/r/-/tree/v1.0/skills/foo"},
		{"gitlab default branch keeps the subdir apart", "https://gitlab.com/g/r/-/tree/v1.0/skills/foo", "", "https://gitlab.com/g/r/-/tree/HEAD/skills/foo"},
		{"gitlab repo root", "https://gitlab.com/g/r.git", "v1.0", "https://gitlab.com/g/r/-/tree/v1.0"},
		{"bitbucket", "https://bitbucket.org/o/r/src/main/skills/foo", "v1.0", "https://bitbucket.org/o/r/src/v1.0/skills/foo"},
		{"gitlab default branch at the root", "https://gitlab.com/g/r/-/tree/v1.0", "", "https://gitlab.com/g/r.git"},
		{"bitbucket default branch at the root", "https://bitbucket.org/o/r/src/v1.0", "", "https://bitbucket.org/o/r.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := ParseSource(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			// Stand in for the remote so no test touches the network.
			refs := RemoteRefs{byURL: map[string]*RefList{
				source.CloneURL: {Branches: []string{"feature/x", "main"}, Tags: []string{"v1.0"}},
			}}
			if err := refs.Settle(source); err != nil {
				t.Fatalf("Settle: %v", err)
			}
			got, err := source.AtRef(tt.ref)
			if err != nil {
				t.Fatalf("AtRef: %v", err)
			}
			if got != tt.want {
				t.Errorf("AtRef(%q) of %q = %q, want %q", tt.ref, tt.raw, got, tt.want)
			}
			if ref := SourceRef(got, ParseOptions{}, &refs); ref != tt.ref {
				t.Errorf("SourceRef(%q) = %q, want %q", got, ref, tt.ref)
			}
		})
	}
}

// The dashboard joins a skill's path onto a repo root pinned at the default
// branch, so that root must keep the repo and the path apart.
func TestSourceAtRef_DefaultBranchRootTakesAPath(t *testing.T) {
	for _, raw := range []string{"https://gitlab.com/g/r/-/tree/v1.0", "https://bitbucket.org/o/r/src/v1.0"} {
		source, err := ParseSource(raw)
		if err != nil {
			t.Fatal(err)
		}
		refs := RemoteRefs{byURL: map[string]*RefList{source.CloneURL: {Tags: []string{"v1.0"}}}}
		if err := refs.Settle(source); err != nil {
			t.Fatalf("Settle: %v", err)
		}
		root, err := source.AtRef("")
		if err != nil {
			t.Fatalf("AtRef: %v", err)
		}
		joined, err := ParseSource(root + "/skills/foo")
		if err != nil {
			t.Fatal(err)
		}
		if joined.CloneURL != source.CloneURL || joined.Subdir != "skills/foo" {
			t.Errorf("%q/skills/foo parsed as repo %q subdir %q, want repo %q subdir skills/foo", root, joined.CloneURL, joined.Subdir, source.CloneURL)
		}
	}
}

func TestSourceAtRef_RejectsOtherHosts(t *testing.T) {
	for _, raw := range []string{"https://git.example.com/o/r", "git@github.com:o/r.git", "/tmp/skills", "file:///tmp/r"} {
		if _, err := SourceAtRef(raw, "v1.0", ParseOptions{}); !errors.Is(err, ErrRefNotPinnable) {
			t.Errorf("SourceAtRef(%q) err = %v, want ErrRefNotPinnable", raw, err)
		}
	}
}
