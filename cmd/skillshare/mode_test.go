package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"skillshare/internal/projectdir"
)

func TestResolveAutoMode(t *testing.T) {
	withProject := t.TempDir()
	cfgDir := filepath.Join(withProject, projectdir.Default)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, projectdir.ConfigFileName), []byte("targets: []\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withoutProject := t.TempDir()

	tests := []struct {
		name string
		mode runMode
		cwd  string
		want runMode
	}{
		{"auto with project config", modeAuto, withProject, modeProject},
		{"auto without project config", modeAuto, withoutProject, modeGlobal},
		{"explicit project ignores missing config", modeProject, withoutProject, modeProject},
		{"explicit global ignores project config", modeGlobal, withProject, modeGlobal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAutoMode(tt.mode, tt.cwd); got != tt.want {
				t.Errorf("resolveAutoMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseModeArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		flags   []string
		mode    runMode
		rest    []string
		wantErr bool
	}{
		{"auto", []string{"path", "--name", "-g"}, []string{"--name"}, modeAuto, []string{"path", "--name", "-g"}, false},
		{"explicit project", []string{"--name", "-g", "-p"}, []string{"--name"}, modeProject, []string{"--name", "-g"}, false},
		{"explicit global", []string{"-g", "--name", "-p"}, []string{"--name"}, modeGlobal, []string{"--name", "-p"}, false},
		{"long value", []string{"--name", "--project", "--global"}, []string{"--name"}, modeGlobal, []string{"--name", "--project"}, false},
		{"repeated values", []string{"--group", "-p", "--group", "-g"}, []string{"--group"}, modeAuto, []string{"--group", "-p", "--group", "-g"}, false},
		{"short option", []string{"-s", "-g", "-p"}, []string{"-s"}, modeProject, []string{"-s", "-g"}, false},
		{"boolean option", []string{"-s", "-g"}, nil, modeGlobal, []string{"-s"}, false},
		{"equals", []string{"--name=-g", "-p"}, []string{"--name"}, modeProject, []string{"--name=-g"}, false},
		{"missing value", []string{"-g", "--name"}, []string{"--name"}, modeGlobal, []string{"--name"}, false},
		{"terminator", []string{"-g", "--", "-p"}, nil, modeGlobal, []string{"--", "-p"}, false},
		{"terminator as value", []string{"--name", "--", "-p"}, []string{"--name"}, modeProject, []string{"--name", "--"}, false},
		{"optional hub", []string{"--hub", "-p"}, []string{"--limit", "-n"}, modeProject, []string{"--hub"}, false},
		{"conflicting modes", []string{"--name", "-g", "-p", "-g"}, []string{"--name"}, modeAuto, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := slices.Clone(tt.args)
			mode, rest, err := parseModeArgs(tt.args, tt.flags...)
			if (err != nil) != tt.wantErr || mode != tt.mode || !slices.Equal(rest, tt.rest) {
				t.Fatalf("got (%v, %q, %v), want (%v, %q, error=%v)", mode, rest, err, tt.mode, tt.rest, tt.wantErr)
			}
			if !slices.Equal(tt.args, original) {
				t.Fatal("modified input arguments")
			}
		})
	}
}
