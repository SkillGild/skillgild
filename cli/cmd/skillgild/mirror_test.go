package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	name, body, link string
	typeflag         byte
	mode             int64
}

func mirrorArchive(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Typeflag: e.typeflag, Linkname: e.link, Mode: mode, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnpackMirrorWritesSkillFiles(t *testing.T) {
	dir := t.TempDir()
	data := mirrorArchive(t,
		tarEntry{name: "SKILL.md", body: "---\nname: demo\n---\n", typeflag: tar.TypeReg},
		tarEntry{name: "scripts/run.sh", body: "echo hi\n", typeflag: tar.TypeReg, mode: 0o755},
		tarEntry{name: "LICENSE", body: "MIT License\n", typeflag: tar.TypeReg},
	)
	if err := unpackMirror(data, dir); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err != nil || !strings.Contains(string(got), "name: demo") {
		t.Fatalf("SKILL.md = %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "scripts", "run.sh"))
	if err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("scripts/run.sh should be executable: %v %v", info, err)
	}
}

func TestUnpackMirrorRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../escape", "/etc/passwd", "a/../../escape"} {
		dir := t.TempDir()
		data := mirrorArchive(t, tarEntry{name: name, body: "x", typeflag: tar.TypeReg})
		if err := unpackMirror(data, dir); err == nil || !strings.Contains(err.Error(), "unsafe path") {
			t.Fatalf("%q: expected an unsafe path error, got %v", name, err)
		}
	}
}

func TestUnpackMirrorSkipsLinks(t *testing.T) {
	dir := t.TempDir()
	data := mirrorArchive(t,
		tarEntry{name: "SKILL.md", body: "x", typeflag: tar.TypeReg},
		tarEntry{name: "secrets", link: "/etc/passwd", typeflag: tar.TypeSymlink},
		tarEntry{name: "hard", link: "SKILL.md", typeflag: tar.TypeLink},
	)
	if err := unpackMirror(data, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"secrets", "hard"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should not be installed: %v", name, err)
		}
	}
}
