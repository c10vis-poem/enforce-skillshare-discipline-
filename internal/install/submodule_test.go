package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newHubWithSubmodule builds a bare hub repo with its own skill at own/ and an
// upstream repo mounted as a submodule at vendor/up (holding skills/a).
func newHubWithSubmodule(t *testing.T) (hubURL, upURL string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmp := t.TempDir()
	commit := func(dir string, files map[string]string) {
		mustRunGit(t, "", "init", "-q", dir)
		mustRunGit(t, dir, "config", "user.email", "test@test.com")
		mustRunGit(t, dir, "config", "user.name", "Test")
		for name, body := range files {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0644); err != nil {
				t.Fatal(err)
			}
		}
		mustRunGit(t, dir, "add", ".")
		mustRunGit(t, dir, "commit", "-q", "-m", "init")
	}

	up := filepath.Join(tmp, "up")
	commit(up, map[string]string{"skills/a/SKILL.md": "# a"})
	upURL = "file://" + filepath.ToSlash(up) // git records forward slashes on Windows too

	work := filepath.Join(tmp, "work")
	commit(work, map[string]string{"own/SKILL.md": "# own"})
	mustRunGit(t, work, "-c", "protocol.file.allow=always", "submodule", "add", "-q", upURL, "vendor/up")
	mustRunGit(t, work, "commit", "-q", "-m", "add submodule")

	hub := filepath.Join(tmp, "hub.git")
	mustRunGit(t, "", "clone", "-q", "--bare", work, hub)
	return "file://" + hub, upURL
}

func hubSource(hubURL, subdir string) *Source {
	return &Source{
		Type:     SourceTypeGitHTTPS,
		Raw:      hubURL + "/" + subdir,
		CloneURL: hubURL,
		Subdir:   subdir,
		Name:     filepath.Base(subdir),
	}
}

func TestDiscoverFromGitSubdir_RefusesSubmodulePaths(t *testing.T) {
	hubURL, upURL := newHubWithSubmodule(t)

	for _, subdir := range []string{"vendor/up", "vendor/up/skills/a", "vendor//up"} {
		t.Run(subdir, func(t *testing.T) {
			result, err := DiscoverFromGitSubdir(hubSource(hubURL, subdir))
			if err == nil {
				CleanupDiscovery(result)
				t.Fatalf("expected an error, got %d skill(s)", len(result.Skills))
			}
			if !strings.Contains(err.Error(), `git submodule "vendor/up"`) || !strings.Contains(err.Error(), upURL) {
				t.Fatalf("error should name the submodule and its upstream, got: %v", err)
			}
		})
	}
}

func TestInstall_RefusesSubmoduleSubdir(t *testing.T) {
	hubURL, _ := newHubWithSubmodule(t)
	dest := filepath.Join(t.TempDir(), "up")

	_, err := Install(hubSource(hubURL, "vendor/up"), dest, InstallOptions{SkipAudit: true})
	if err == nil || !strings.Contains(err.Error(), "git submodule") {
		t.Fatalf("expected a submodule error, got: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("nothing should be installed at %s", dest)
	}
}

func TestSubmoduleError_CaseVariantFollowsFilesystem(t *testing.T) {
	hubURL, _ := newHubWithSubmodule(t)
	repo := filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, "", "clone", "-q", hubURL, repo) // leaves vendor/up as an empty dir

	_, statErr := os.Stat(filepath.Join(repo, "Vendor", "Up"))
	foldsCase := statErr == nil
	if err := submoduleError(repo, "Vendor/Up", nil); (err != nil) != foldsCase {
		t.Fatalf("case-insensitive filesystem = %v, but submoduleError(Vendor/Up) = %v", foldsCase, err)
	}
}

func TestInstallTrackedRepo_RefusesSubmoduleSubdir(t *testing.T) {
	hubURL, _ := newHubWithSubmodule(t)
	sourceDir := t.TempDir()

	_, err := InstallTrackedRepo(hubSource(hubURL, "vendor/up"), sourceDir, InstallOptions{Name: "up", Force: true})
	if err == nil || !strings.Contains(err.Error(), "git submodule") {
		t.Fatalf("expected a submodule error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(sourceDir, "_up")); !os.IsNotExist(statErr) {
		t.Fatalf("the refused tracked clone should be removed, stat err = %v", statErr)
	}
}

func TestSubmoduleError_FailsClosedWhenTreeUnreadable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir()) // keep git from finding an enclosing repo
	if err := submoduleError(t.TempDir(), "vendor/up", nil); err == nil {
		t.Fatal("expected an error when the tree cannot be listed")
	}
}

func TestInstallTrackedRepo_UpdateWarnsSkippedSubmodule(t *testing.T) {
	hubURL, _ := newHubWithSubmodule(t)
	sourceDir := t.TempDir()
	source := &Source{Type: SourceTypeGitHTTPS, Raw: hubURL, CloneURL: hubURL, Name: "hub"}
	if _, err := InstallTrackedRepo(source, sourceDir, InstallOptions{Name: "hub"}); err != nil {
		t.Fatal(err)
	}

	result, err := InstallTrackedRepo(source, sourceDir, InstallOptions{Name: "hub", Update: true, SkipAudit: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "updated" || !strings.Contains(strings.Join(result.Warnings, "\n"), `skipped git submodule "vendor/up"`) {
		t.Fatalf("expected an update with a submodule warning, got action %q warnings %q", result.Action, result.Warnings)
	}
}

func TestGitlinkString_EscapesControlCharacters(t *testing.T) {
	got := gitlink{Path: "vendor/\x1b]52;c;Zm9v\x07up\n✓ fake", Commit: "abc", URL: "https://example.com/\x1b[31mup.git"}.String()
	if strings.ContainsAny(got, "\x1b\x07\n") {
		t.Fatalf("String() = %q, want control characters escaped", got)
	}
}

func TestGitlinkString_HidesURLCredentials(t *testing.T) {
	for raw, want := range map[string]string{
		"https://user:s3cret@example.com/org/up.git":         "https://example.com/org/up.git",
		"https://s3cret@example.com/org/up.git?token=s3cret": "https://example.com/org/up.git",
		"https://user:s3 cret@example.com/org/up.git#s3cret": "https://example.com/org/up.git",
		"../up.git?token=s3cret":                             "../up.git",
		"s3cret@example.com:org/up.git":                      "example.com:org/up.git",
		"https:/user:s3cret@example.com/org/up.git":          "example.com/org/up.git",
		"//user:s3cret@example.com/org/up.git":               "example.com/org/up.git",
	} {
		got := gitlink{Path: "vendor/up", Commit: "abc", URL: raw}.String()
		if strings.Contains(got, "s3cret") || !strings.Contains(got, want) {
			t.Errorf("String() for %q = %q, want %q without credentials", raw, got, want)
		}
	}
}

func TestDiscoverFromGit_WarnsSkippedSubmodule(t *testing.T) {
	hubURL, upURL := newHubWithSubmodule(t)

	result, err := DiscoverFromGit(&Source{Type: SourceTypeGitHTTPS, Raw: hubURL, CloneURL: hubURL, Name: "hub"})
	if err != nil {
		t.Fatal(err)
	}
	defer CleanupDiscovery(result)

	if len(result.Skills) != 1 || result.Skills[0].Name != "own" {
		t.Fatalf("expected only the hub's own skill, got %+v", result.Skills)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], `skipped git submodule "vendor/up"`) || !strings.Contains(result.Warnings[0], upURL) {
		t.Fatalf("expected one submodule warning, got %q", result.Warnings)
	}
}
