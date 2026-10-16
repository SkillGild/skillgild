package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Releases come from the public repository the install scripts use. Tests point these
// at a local server.
var (
	releaseAPI   = "https://api.github.com/repos/SkillGild/skillgild/releases/latest"
	downloadBase = "https://github.com/SkillGild/skillgild/releases/download"
)

// errNeedsNewer marks an error that a newer CLI may not have: an unknown command,
// --agent preset or connect target. main updates and retries once on it.
var errNeedsNewer = errors.New("if it is new, `skillgild update` gets the latest version")

// autoUpdateAllowed is false for development builds and when the user opted out.
func autoUpdateAllowed() bool {
	return version != "dev" && !strings.HasSuffix(version, "-dev") && os.Getenv("SKILLGILD_NO_AUTO_UPDATE") == ""
}

// newerVersion reports whether release a is newer than b ("v0.1.9" vs "0.1.8").
func newerVersion(a, b string) bool {
	parse := func(v string) [3]int {
		var out [3]int
		v = strings.TrimPrefix(strings.SplitN(v, "-", 2)[0], "v")
		for i, part := range strings.SplitN(v, ".", 3) {
			out[i], _ = strconv.Atoi(part)
		}
		return out
	}
	x, y := parse(a), parse(b)
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

func latestVersion(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "skillgild-cli/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check for updates: GitHub answered %s", resp.Status)
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil || release.Tag == "" {
		return "", fmt.Errorf("check for updates: unreadable release")
	}
	return release.Tag, nil
}

func fetch(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "skillgild-cli/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// selfUpdate replaces the executable at exe with the latest release when it is newer
// than the running version. It returns the version installed, or "" when already current.
func selfUpdate(ctx context.Context, client *http.Client, exe string) (string, error) {
	latest, err := latestVersion(ctx, client)
	if err != nil {
		return "", err
	}
	if !newerVersion(latest, version) {
		return "", nil
	}
	plain := strings.TrimPrefix(latest, "v")
	ext, binary := "tar.gz", "skillgild"
	if runtime.GOOS == "windows" {
		ext, binary = "zip", "skillgild.exe"
	}
	archive := fmt.Sprintf("skillgild_%s_%s_%s.%s", plain, runtime.GOOS, runtime.GOARCH, ext)
	base := downloadBase + "/" + latest + "/"
	sums, err := fetch(ctx, client, base+"checksums.txt", 1<<20)
	if err != nil {
		return "", err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == archive {
			want = fields[0]
		}
	}
	if want == "" {
		return "", fmt.Errorf("release %s has no build for %s/%s", latest, runtime.GOOS, runtime.GOARCH)
	}
	data, err := fetch(ctx, client, base+archive, 200<<20)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		return "", fmt.Errorf("downloaded %s does not match its checksum; nothing was changed", archive)
	}
	bin, err := extractBinary(data, ext, binary)
	if err != nil {
		return "", err
	}
	if err := replaceExecutable(exe, bin); err != nil {
		return "", err
	}
	return latest, nil
}

func extractBinary(data []byte, ext, name string) ([]byte, error) {
	if ext == "zip" {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("read update archive: %w", err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 200<<20))
			}
		}
		return nil, fmt.Errorf("update archive has no %s", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read update archive: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("update archive has no %s", name)
		}
		if err != nil {
			return nil, fmt.Errorf("read update archive: %w", err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}

// replaceExecutable swaps in the new binary next to the old one, so a failure leaves the
// old one in place. Windows cannot overwrite a running program but can rename it, so
// the old file is moved aside first and removed on a later run.
func replaceExecutable(exe string, bin []byte) error {
	dir := filepath.Dir(exe)
	next := exe + ".new"
	if err := os.WriteFile(next, bin, 0o755); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("no permission to update %s; rerun the install script (with sudo if it lives in a system folder)", dir)
		}
		return fmt.Errorf("write update: %w", err)
	}
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(next)
			return fmt.Errorf("move the old skillgild aside: %w", err)
		}
		if err := os.Rename(next, exe); err != nil {
			os.Rename(old, exe)
			return fmt.Errorf("install update: %w", err)
		}
		return nil
	}
	if err := os.Rename(next, exe); err != nil {
		os.Remove(next)
		return fmt.Errorf("install update: %w", err)
	}
	return nil
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// cleanupOldExecutable removes the copy a Windows update moved aside.
func cleanupOldExecutable() {
	if runtime.GOOS != "windows" {
		return
	}
	if exe, err := executablePath(); err == nil {
		os.Remove(exe + ".old")
	}
}

// updateCommand is `skillgild update`.
func updateCommand(ctx context.Context) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	installed, err := selfUpdate(ctx, &http.Client{Timeout: 2 * time.Minute}, exe)
	if err != nil {
		return err
	}
	if installed == "" {
		fmt.Printf("skillgild %s is the latest version.\n", version)
		return nil
	}
	fmt.Printf("Updated skillgild %s to %s.\n", version, installed)
	return nil
}

// updateAndRetry updates the CLI and runs the same command again with the new version.
// It returns false when nothing newer is available, so the caller reports the error.
func updateAndRetry(ctx context.Context, args []string) bool {
	if !autoUpdateAllowed() {
		return false
	}
	exe, err := executablePath()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	installed, err := selfUpdate(ctx, &http.Client{Timeout: 2 * time.Minute}, exe)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skillgild: could not update automatically:", err)
		return false
	}
	if installed == "" {
		return false
	}
	fmt.Fprintf(os.Stderr, "skillgild: updated %s to %s; running your command again.\n", version, installed)
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "SKILLGILD_NO_AUTO_UPDATE=1")
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "skillgild:", err)
		os.Exit(1)
	}
	os.Exit(0)
	return true
}
