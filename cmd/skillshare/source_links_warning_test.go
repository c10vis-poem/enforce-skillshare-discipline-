package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillshare/internal/config"
)

func TestSourceLinkWarningsListAndStatus(t *testing.T) {
	for _, project := range []bool{false, true} {
		for _, jsonOutput := range []bool{false, true} {
			for _, command := range []string{"list", "status"} {
				t.Run(fmt.Sprintf("%s/project=%t/json=%t", command, project, jsonOutput), func(t *testing.T) {
					root := t.TempDir()
					source := filepath.Join(root, "skills")
					t.Setenv("SKILLSHARE_CONFIG", filepath.Join(root, "config.yaml"))
					cfg := &config.Config{Source: source, FollowSourceLinks: true}
					if err := cfg.Save(); err != nil {
						t.Fatal(err)
					}
					if project {
						pc := &config.ProjectConfig{Sources: config.ProjectSources{Skills: source}, FollowSourceLinks: true}
						if err := pc.Save(root); err != nil {
							t.Fatal(err)
						}
					}
					setupUpdatableSkill(t, source, "healthy")
					if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(source, "gone")); err != nil {
						t.Fatal(err)
					}
					output := captureStdoutStderr(t, func() {
						var err error
						switch {
						case command == "list" && project:
							err = cmdListProject(root, listOptions{NoTUI: true, JSON: jsonOutput}, kindSkills)
						case command == "list":
							args := []string{"-g", "--no-tui"}
							if jsonOutput {
								args = append(args, "--json")
							}
							err = cmdList(args)
						case project && jsonOutput:
							err = cmdStatusProjectJSON(root)
						case project:
							err = cmdStatusProject(root)
						default:
							args := []string{"-g"}
							if jsonOutput {
								args = append(args, "--json")
							}
							err = cmdStatus(args)
						}
						if err != nil {
							t.Errorf("%s failed: %v", command, err)
						}
					})
					if !strings.Contains(output, "source link gone not followed: target is missing") {
						t.Fatalf("missing warning: %s", output)
					}
					if command == "list" && !strings.Contains(output, "healthy") {
						t.Fatalf("healthy skill disappeared: %s", output)
					}
					if command == "status" && !strings.Contains(output, "1") {
						t.Fatalf("healthy inventory disappeared: %s", output)
					}
				})
			}
		}
	}
}
