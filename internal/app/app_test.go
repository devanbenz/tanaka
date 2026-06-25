package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirCreatesUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if dir != filepath.Join(home, ".tanaka") {
		t.Fatalf("dir = %q", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("data dir not created: %v", err)
	}
}

func TestNewIDUniqueAndHex(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b {
		t.Fatal("ids not unique")
	}
	if len(a) != 16 {
		t.Fatalf("id len = %d, want 16", len(a))
	}
	if strings.ContainsAny(a, "ghijklmnopqrstuvwxyz") {
		t.Fatalf("id %q is not hex", a)
	}
	if strings.ContainsAny(b, "ghijklmnopqrstuvwxyz") {
		t.Fatalf("id %q is not hex", b)
	}
}

func TestDBPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := DBPath()
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(home, ".tanaka", "tanaka.db") {
		t.Fatalf("DBPath = %q", p)
	}
}
