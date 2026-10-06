//go:build !online

package integration

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"skillshare/internal/testutil"
)

const serveMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}`

func TestMCPServe_StdioListsValidSkills(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")
	sb.CreateSkill("pdf", map[string]string{"SKILL.md": "---\nname: pdf\ndescription: Use when editing PDFs\n---\n# PDF\n"})
	sb.CreateSkill("renamed", map[string]string{"SKILL.md": "---\nname: original\ndescription: Use when testing\n---\n# Renamed\n"})

	// The server drops requests still in flight when stdin closes, so keep it
	// open until both responses arrive, as a spawning gateway does.
	cmd := exec.Command(sb.BinaryPath, "mcp", "serve", "-g")
	cmd.Env = append(os.Environ(), "HOME="+sb.Home, "SKILLSHARE_CONFIG="+sb.ConfigPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+serveMeta+`}}`)
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":2,"method":"skills/list","params":{`+serveMeta+`}}`)
	scanner := bufio.NewScanner(stdout)
	var out strings.Builder
	for range 2 {
		if !scanner.Scan() {
			t.Fatalf("missing response: %v\nstderr: %s", scanner.Err(), stderr.String())
		}
		out.WriteString(scanner.Text())
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("serve exited with %v\nstderr: %s", err, stderr.String())
	}

	for _, want := range []string{`"io.modelcontextprotocol/skills"`, `"uri":"skill://pdf/SKILL.md"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("responses missing %s:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "skill://renamed") {
		t.Errorf("renamed skill was served:\n%s", out.String())
	}
	if want := `skipped renamed: SKILL.md name "original" does not match directory name "renamed"`; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr missing %q:\n%s", want, stderr.String())
	}
}

func TestMCPServe_CheckListsSkippedSkillsWithoutServing(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")
	sb.CreateSkill("pdf", map[string]string{"SKILL.md": "---\nname: pdf\ndescription: Use when editing PDFs\n---\n# PDF\n"})
	sb.CreateSkill("renamed", map[string]string{"SKILL.md": "---\nname: original\ndescription: Use when testing\n---\n# Renamed\n"})

	r := sb.RunCLI("mcp", "serve", "--check", "-g")

	r.AssertSuccess(t)
	r.AssertRowContains(t, "renamed", `SKILL.md name "original" does not match directory name "renamed"`)
	r.AssertOutputContains(t, "1 skill served, 1 skipped")
}

func TestMCPServe_RefusesUnauthenticatedNetworkListener(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")

	r := sb.RunCLI("mcp", "serve", "--http", "0.0.0.0:0")

	r.AssertFailure(t)
	r.AssertAnyOutputContains(t, "SKILLSHARE_MCP_TOKEN")
}

func TestMCPServe_RejectsUnknownTarget(t *testing.T) {
	sb := testutil.NewSandbox(t)
	defer sb.Cleanup()
	sb.WriteConfig("source: " + sb.SourcePath + "\ntargets: {}\n")

	r := sb.RunCLI("mcp", "serve", "--target", "nope")

	r.AssertFailure(t)
	r.AssertAnyOutputContains(t, `unknown target "nope"`)
}
