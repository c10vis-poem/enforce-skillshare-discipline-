package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
	ssync "skillshare/internal/sync"
)

// attachShared creates the shared file team and attaches it to target.
func attachShared(t *testing.T, s *Server, target string) {
	t.Helper()
	if rr := instructionsRequest(t, s, http.MethodPost, "/api/instructions", `{"name":"team","content":"team\n"}`); rr.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	if res := decodeBody[map[string]any](t, instructionsRequest(t, s, http.MethodPost, "/api/instructions/assign", `{"targets":["`+target+`"],"extras":["team"]}`)); res["success"] != true {
		t.Fatalf("assign: %v", res)
	}
}

func TestSharedInstructionsMode_SwitchKeepsRestorePoint(t *testing.T) {
	s, home := newInstructionsServer(t, "claude")
	claude := writeHome(t, home, ".claude/CLAUDE.md", "mine\n")
	attachShared(t, s, "claude")

	if rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/claude/mode", `{"mode":"copy"}`); rr.Code != http.StatusOK {
		t.Fatalf("mode: %d %s", rr.Code, rr.Body.String())
	}
	if got := readFile(t, claude); got != "team\n" {
		t.Fatalf("after copy CLAUDE.md = %q, want the shared file", got)
	}
	if res := decodeBody[map[string]any](t, instructionsRequest(t, s, http.MethodPost, "/api/instructions/team/restore", `{"target":"claude"}`)); res["success"] != true {
		t.Fatalf("restore: %v", res)
	}
	if got := readFile(t, claude); got != "mine\n" {
		t.Errorf("restored CLAUDE.md = %q, want the file from before attaching", got)
	}
}

func TestSharedInstructionsMode_ImportNeedsImportTarget(t *testing.T) {
	s, _ := newInstructionsServer(t, "codex")
	attachShared(t, s, "codex")

	rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/codex/mode", `{"mode":"import"}`)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSharedInstructionsMode_SymlinkNeedsFileLinks(t *testing.T) {
	t.Cleanup(ssync.SetFileLinksForTest(false))
	s, _ := newInstructionsServer(t, "codex")
	attachShared(t, s, "codex")

	list := decodeBody[map[string]any](t, instructionsRequest(t, s, http.MethodGet, "/api/instructions", ""))
	rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/codex/mode", `{"mode":"symlink"}`)
	if list["file_links"] != false || rr.Code != http.StatusBadRequest {
		t.Errorf("file_links = %v, symlink status = %d; want false and 400", list["file_links"], rr.Code)
	}
}

// A tool without @import can switch from the link to a managed block: its own
// file comes back with the shared content in a block at the top.
func TestSharedInstructionsMode_PrependKeepsOwnFile(t *testing.T) {
	s, home := newInstructionsServer(t, "codex")
	codex := writeHome(t, home, ".codex/AGENTS.md", "mine\n")
	attachShared(t, s, "codex")

	if rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/codex/mode", `{"mode":"prepend"}`); rr.Code != http.StatusOK {
		t.Fatalf("mode: %d %s", rr.Code, rr.Body.String())
	}
	got := readFile(t, codex)
	if !strings.HasPrefix(got, `<!-- skillshare:extra src="`) || !strings.HasSuffix(got, "team\n<!-- /skillshare:extra -->\n\nmine\n") {
		t.Fatalf("after prepend AGENTS.md =\n%s", got)
	}
	if res := decodeBody[map[string]any](t, instructionsRequest(t, s, http.MethodPost, "/api/instructions/team/restore", `{"target":"codex"}`)); res["success"] != true {
		t.Fatalf("restore: %v", res)
	}
	if got := readFile(t, codex); got != "mine\n" {
		t.Errorf("restored AGENTS.md = %q, want only the file's own content", got)
	}
}

func TestSharedInstructionsRestorePreview_ReplacedFile(t *testing.T) {
	s, home := newInstructionsServer(t, "codex")
	writeHome(t, home, ".codex/AGENTS.md", "mine\n")
	attachShared(t, s, "codex")

	got := decodeBody[sharedRestorePreview](t, instructionsRequest(t, s, http.MethodGet, "/api/instructions/team/restore-preview?target=codex", ""))
	if got.Kind != ssync.RestoreKindContent || got.Content != "mine\n" || got.Current != "team\n" || got.RecordedAt == nil || got.Drift {
		t.Errorf("preview = %+v, want the replaced file", got)
	}
}

func TestSharedInstructionsRestorePreview_NoFileBefore(t *testing.T) {
	s, _ := newInstructionsServer(t, "codex")
	attachShared(t, s, "codex")

	got := decodeBody[sharedRestorePreview](t, instructionsRequest(t, s, http.MethodGet, "/api/instructions/team/restore-preview?target=codex", ""))
	if got.Kind != ssync.RestoreKindDelete {
		t.Errorf("kind = %q, want delete", got.Kind)
	}
}

func TestSharedInstructionsRestorePreview_ImportKeepsOwnLines(t *testing.T) {
	s, home := newInstructionsServer(t, "claude")
	writeHome(t, home, ".claude/CLAUDE.md", "mine\n")
	attachShared(t, s, "claude")

	got := decodeBody[sharedRestorePreview](t, instructionsRequest(t, s, http.MethodGet, "/api/instructions/team/restore-preview?target=claude", ""))
	if got.Kind != ssync.RestoreKindContent || got.Content != "mine\n" {
		t.Errorf("preview = %+v, want the file without the import block", got)
	}
}

func TestSharedInstructionsRestorePreview_EditedTargetIsDrift(t *testing.T) {
	s, home := newInstructionsServer(t, "codex")
	attachShared(t, s, "codex")
	if rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/codex/mode", `{"mode":"copy"}`); rr.Code != http.StatusOK {
		t.Fatalf("mode: %d %s", rr.Code, rr.Body.String())
	}
	writeHome(t, home, ".codex/AGENTS.md", "edited in codex\n")

	got := decodeBody[sharedRestorePreview](t, instructionsRequest(t, s, http.MethodGet, "/api/instructions/team/restore-preview?target=codex", ""))
	if !got.Drift {
		t.Errorf("drift = false, want the edit reported as backed up")
	}
}

func TestTargetInstructions_DefaultPathWithCustomLocation(t *testing.T) {
	s, home := newInstructionsServer(t, "codex")
	if rr := instructionsRequest(t, s, http.MethodPut, "/api/targets/codex/instructions/setup", `{"path":"~/.codex/instructions.md"}`); rr.Code != http.StatusOK {
		t.Fatalf("setup: %d %s", rr.Code, rr.Body.String())
	}

	got := decodeBody[instructionsFileResponse](t, instructionsRequest(t, s, http.MethodGet, "/api/targets/codex/instructions", ""))
	if got.DefaultPath != filepath.Join(home, ".codex", "AGENTS.md") || !strings.HasSuffix(got.Path, "instructions.md") {
		t.Errorf("default_path = %q, path = %q", got.DefaultPath, got.Path)
	}
}

func TestSharedInstructionsSave_UpdatesCopies(t *testing.T) {
	s, home := newInstructionsServer(t, "codex")
	attachShared(t, s, "codex")
	if rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/codex/mode", `{"mode":"copy"}`); rr.Code != http.StatusOK {
		t.Fatalf("mode: %d %s", rr.Code, rr.Body.String())
	}

	res := decodeBody[struct {
		Copies []sharedCopyResult `json:"copies"`
	}](t, instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/content", `{"content":"new\n"}`))
	if got := readFile(t, filepath.Join(home, ".codex", "AGENTS.md")); got != "new\n" || len(res.Copies) != 1 || res.Copies[0].Target != "codex" {
		t.Errorf("codex copy = %q, copies = %+v", got, res.Copies)
	}
}

func TestSharedInstructionsSave_BacksUpEditedCopy(t *testing.T) {
	s, home := newInstructionsServer(t, "codex")
	attachShared(t, s, "codex")
	if rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/codex/mode", `{"mode":"copy"}`); rr.Code != http.StatusOK {
		t.Fatalf("mode: %d %s", rr.Code, rr.Body.String())
	}
	writeHome(t, home, ".codex/AGENTS.md", "edited in codex\n")

	res := decodeBody[struct {
		Copies []sharedCopyResult `json:"copies"`
	}](t, instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/content", `{"content":"new\n"}`))

	got := readFile(t, filepath.Join(home, ".codex", "AGENTS.md"))
	if got != "new\n" || len(res.Copies) != 1 || len(res.Copies[0].Warnings) != 1 || !strings.Contains(res.Copies[0].Warnings[0], "backed up") {
		t.Errorf("codex copy = %q, copies = %+v; want the edit backed up, then replaced", got, res.Copies)
	}
}

func TestSharedInstructionsMode_DirectoryConflictPreservesModeAndAllowsRetry(t *testing.T) {
	s, home := newInstructionsServer(t, "claude")
	attachShared(t, s, "claude")
	p := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0755); err != nil {
		t.Fatal(err)
	}
	rr := instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/claude/mode", `{"mode":"copy"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s; want 409", rr.Code, rr.Body.String())
	}
	body := decodeBody[map[string]any](t, rr)
	if diagnostic, ok := body["error"].(string); !ok || !strings.Contains(diagnostic, "is a directory; not replaced") {
		t.Fatalf("missing conflict diagnostic: %v", body)
	}
	if s.cfg.Extras[0].Targets[0].Mode != "import" {
		t.Fatal("failed transition changed in-memory mode")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Extras[0].Targets[0].Mode != "import" {
		t.Fatal("failed transition persisted mode")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	rr = instructionsRequest(t, s, http.MethodPut, "/api/instructions/team/targets/claude/mode", `{"mode":"copy"}`)
	if rr.Code != http.StatusOK || readFile(t, p) != "team\n" || s.cfg.Extras[0].Targets[0].Mode != "copy" {
		t.Fatalf("retry failed: %d %s", rr.Code, rr.Body.String())
	}
}
