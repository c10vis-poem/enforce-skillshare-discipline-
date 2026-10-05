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
