package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleCheck_FollowedCheckoutsInformational(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("project=%t/stream=%t", project, stream), func(t *testing.T) {
				s, source := newUpdateFollowServer(t, project, true)
				checkout := t.TempDir()
				initGitRepo(t, checkout)
				addSkill(t, checkout, "child")
				for _, name := range []string{"_dev", "dev"} {
					if err := os.Symlink(checkout, filepath.Join(source, name)); err != nil {
						t.Fatal(err)
					}
				}
				addTrackedRepoWithBrokenIndex(t, source, "_managed")
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
				if stream {
					s.handleCheckStream(rr, req)
				} else {
					s.handleCheck(rr, req)
				}
				data := rr.Body.String()
				if stream {
					_, done, ok := strings.Cut(data, "event: done\n")
					if !ok {
						t.Fatalf("no done event: %s", data)
					}
					data = strings.TrimSpace(strings.TrimPrefix(done, "data: "))
				}
				var resp struct {
					Tracked []repoCheckResult `json:"tracked_repos"`
					Linked  []struct {
						Name   string `json:"name"`
						Target string `json:"target"`
					} `json:"linked_repos"`
				}
				if err := json.Unmarshal([]byte(data), &resp); err != nil {
					t.Fatal(err)
				}
				if len(resp.Tracked) != 1 || resp.Tracked[0].Name != "_managed" || resp.Tracked[0].Status != "error" {
					t.Fatalf("tracked repos changed: %+v", resp.Tracked)
				}
				if len(resp.Linked) != 2 {
					t.Fatalf("followed checkouts missing: %s", data)
				}
				for _, link := range resp.Linked {
					if link.Target != checkout {
						t.Fatalf("wrong link target: %+v", link)
					}
				}
				if _, err := os.Stat(filepath.Join(checkout, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
					t.Fatalf("followed checkout was fetched: %v", err)
				}
				rr = httptest.NewRecorder()
				s.handleListSkills(rr, httptest.NewRequest(http.MethodGet, "/api/resources", nil))
				var list struct {
					Linked []any `json:"linked_repos"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
					t.Fatal(err)
				}
				if len(list.Linked) != 2 {
					t.Fatalf("initial list lacks linked checkout identity: %s", rr.Body.String())
				}
			})
		}
	}
}
