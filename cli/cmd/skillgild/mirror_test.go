package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkillGild/skillgild/cli/internal/agentclient"
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

func TestInstallHostedEngineReplacesSkillMD(t *testing.T) {
	data := mirrorArchive(t,
		tarEntry{name: "SKILL.md", body: "upstream instructions"},
		tarEntry{name: "scripts/engine.py", body: "print('ok')", mode: 0o755},
	)
	sum := sha256.Sum256(data)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/skills/last30days/source" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	api, err := agentclient.New(server.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	skill := agentclient.Skill{ID: "skill-id", Slug: "last30days", Name: "last30days", Description: "Research", AccessTier: "free", RuntimeType: "hybrid_tools",
		DistributionMode: "hosted_public", Mirror: &agentclient.Mirror{Commit: "5103ba478b380552", SHA256: hex.EncodeToString(sum[:])}}
	dir := filepath.Join(t.TempDir(), "last30days")
	if err := installHostedEngine(context.Background(), api, skill, dir, false); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(dir, "scripts", "engine.py")); err != nil || string(body) != "print('ok')" {
		t.Fatalf("engine script not installed: %q %v", body, err)
	}
	wrapperText, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wrapperText), "upstream instructions") || !strings.Contains(string(wrapperText), "skillgild_start_session") || !strings.Contains(string(wrapperText), "`SKILL_DIR`") {
		t.Fatalf("SKILL.md should be the hosted wrapper naming SKILL_DIR, got:\n%s", wrapperText)
	}

	skill.Mirror.SHA256 = strings.Repeat("0", 64)
	if err := installHostedEngine(context.Background(), api, skill, filepath.Join(t.TempDir(), "x"), false); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want a checksum error, got %v", err)
	}
}
