//go:build windows

package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateJunctionPathWithCmdMetacharacters(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target&more%TEMP%")
	os.MkdirAll(target, 0755)
	link := filepath.Join(base, "link&echo%TEMP%")

	if err := CreateJunction(link, target); err != nil {
		t.Fatal(err)
	}
	if !IsJunction(link) {
		t.Fatal("link is not a junction")
	}
	if got, err := ResolveLinkTarget(link); err != nil || got != target {
		t.Fatalf("target = %q, %v; want %q", got, err, target)
	}
}

func TestCreateJunctionExtendedPathTarget(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	os.MkdirAll(target, 0755)
	os.WriteFile(filepath.Join(target, "marker"), []byte("x"), 0644)
	link := filepath.Join(base, "link")

	if err := CreateJunction(link, `\\?\`+target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(link, "marker")); err != nil {
		t.Fatalf("cannot read through the junction: %v", err)
	}
}
