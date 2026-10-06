//go:build windows

package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"skillshare/internal/utils"
)

func crossDevice(string, string) error { return fmt.Errorf("cross-device rename") }

func TestMoveJunctionAcrossVolumesStaysJunction(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "checkout")
	source := filepath.Join(base, "source")
	os.MkdirAll(target, 0755)
	os.MkdirAll(source, 0755)
	link := filepath.Join(source, "_dev-skills")
	if err := utils.CreateJunction(link, target); err != nil {
		t.Skip(err)
	}

	trashed, err := moveToTrash(link, "_dev-skills", filepath.Join(base, "trash"), crossDevice)
	if err != nil {
		t.Fatal(err)
	}
	if !utils.IsJunction(trashed) {
		t.Fatalf("trashed entry is not a junction")
	}
	items := List(filepath.Join(base, "trash"))
	if len(items) != 1 {
		t.Fatalf("list = %+v", items)
	}
	if err := restore(&items[0], source, crossDevice); err != nil {
		t.Fatal(err)
	}
	if !utils.IsJunction(link) {
		t.Fatalf("restored entry is not a junction")
	}
	if got, err := utils.ResolveLinkTarget(link); err != nil || got != target {
		t.Fatalf("restored target = %q, %v; want %q", got, err, target)
	}
}

func TestMoveJunctionFailureKeepsOriginal(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "checkout")
	os.MkdirAll(target, 0755)
	link := filepath.Join(base, "link")
	if err := utils.CreateJunction(link, target); err != nil {
		t.Skip(err)
	}
	dst := filepath.Join(base, "occupied")
	os.WriteFile(dst, nil, 0644) // mklink /J refuses an existing destination

	if err := moveLink(link, dst, crossDevice); err == nil {
		t.Fatal("expected an error")
	}
	if !utils.IsJunction(link) {
		t.Fatal("original junction was not kept")
	}
}
