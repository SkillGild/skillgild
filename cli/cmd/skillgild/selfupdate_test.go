package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"v0.1.9", "0.1.8", true}, {"v0.1.8", "0.1.8", false}, {"v0.2.0", "0.1.12", true}, {"v0.1.10", "0.1.9", true}, {"v0.1.7", "0.1.8", false}, {"v1.0.0", "0.9.9-dev", true}} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("%s > %s: got %v", c.a, c.b, got)
		}
	}
}

func fakeArchive(t *testing.T, payload []byte) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("skillgild.exe")
		w.Write(payload)
		zw.Close()
		return "zip", buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("hi"))
	tw.WriteHeader(&tar.Header{Name: "skillgild", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg})
	tw.Write(payload)
	tw.Close()
	gz.Close()
	return "tar.gz", buf.Bytes()
}

func TestSelfUpdate(t *testing.T) {
	old := version
	version = "0.1.8"
	t.Cleanup(func() { version = old })
	payload := []byte("new skillgild binary")
	ext, archive := fakeArchive(t, payload)
	name := fmt.Sprintf("skillgild_0.1.9_%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
	sum := sha256.Sum256(archive)
	tamper := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprint(w, `{"tag_name":"v0.1.9"}`)
		case "/v0.1.9/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		case "/v0.1.9/" + name:
			if tamper {
				w.Write(append([]byte("x"), archive...))
				return
			}
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	releaseAPI, downloadBase = srv.URL+"/latest", srv.URL
	exe := filepath.Join(t.TempDir(), "skillgild")
	os.WriteFile(exe, []byte("old binary"), 0o755)

	// A tampered download changes nothing.
	tamper = true
	if _, err := selfUpdate(context.Background(), srv.Client(), exe); err == nil {
		t.Fatal("accepted a download that fails its checksum")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old binary" {
		t.Fatal("binary changed after a failed update")
	}

	tamper = false
	installed, err := selfUpdate(context.Background(), srv.Client(), exe)
	if err != nil || installed != "v0.1.9" {
		t.Fatalf("update: %q %v", installed, err)
	}
	if b, _ := os.ReadFile(exe); !bytes.Equal(b, payload) {
		t.Fatalf("binary not replaced: %q", b)
	}

	// Already current: nothing to do.
	version = "0.1.9"
	if installed, err := selfUpdate(context.Background(), srv.Client(), exe); err != nil || installed != "" {
		t.Fatalf("current: %q %v", installed, err)
	}
}
