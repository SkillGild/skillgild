package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

func testOnboarding(t *testing.T, binaries ...string) *onboarding {
	t.Helper()
	home := t.TempDir()
	return &onboarding{home: home, exe: filepath.Join(home, "bin", "skillgild"), preferencePath: filepath.Join(home, "config", "setup.json"), in: strings.NewReader(""), out: io.Discard,
		lookup: func(name string) (string, error) {
			for _, b := range binaries {
				if b == name {
					return filepath.Join(home, "bin", name), nil
				}
			}
			return "", execMissing
		},
		authenticate: func(context.Context, string, bool) error { return nil }, register: func(context.Context, string) error { return nil }}
}

var execMissing = errors.New("executable not found")

func TestAgentSelection(t *testing.T) {
	t.Run("single CLI", func(t *testing.T) {
		s := testOnboarding(t, "codex")
		got, err := s.selectAgent("auto")
		if err != nil || got != "codex" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("IDE directory", func(t *testing.T) {
		s := testOnboarding(t)
		os.Mkdir(filepath.Join(s.home, ".cursor"), 0755)
		got, err := s.selectAgent("auto")
		if err != nil || got != "cursor" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("shared Gemini directory is not CLI evidence", func(t *testing.T) {
		s := testOnboarding(t)
		os.Mkdir(filepath.Join(s.home, ".gemini"), 0755)
		if _, err := s.selectAgent("auto"); err == nil {
			t.Fatal("detected Gemini without its CLI")
		}
	})
	t.Run("multiple agents in scripts", func(t *testing.T) {
		s := testOnboarding(t, "claude", "codex")
		if _, err := s.selectAgent("auto"); err == nil || !strings.Contains(err.Error(), "--agent") {
			t.Fatalf("%v", err)
		}
	})
	t.Run("interactive choice and invalid input", func(t *testing.T) {
		s := testOnboarding(t, "claude", "codex")
		s.interactive = true
		s.in = strings.NewReader("bad\n2\n")
		got, err := s.selectAgent("auto")
		if err != nil || got != "codex" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("EOF cancels", func(t *testing.T) {
		s := testOnboarding(t, "claude", "codex")
		s.interactive = true
		if _, err := s.selectAgent("auto"); err == nil {
			t.Fatal("EOF accepted")
		}
	})
	t.Run("remembered agent", func(t *testing.T) {
		s := testOnboarding(t, "claude", "codex")
		if err := s.prepare(context.Background(), "api", "codex"); err != nil {
			t.Fatal(err)
		}
		got, err := s.selectAgent("auto")
		if err != nil || got != "codex" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("missing agent CLI", func(t *testing.T) {
		s := testOnboarding(t)
		if _, err := s.selectAgent("claude-code"); err == nil {
			t.Fatal("accepted missing CLI")
		}
	})
	t.Run("explicit JSON client", func(t *testing.T) {
		s := testOnboarding(t)
		got, err := s.selectAgent("cursor")
		if err != nil || got != "cursor" {
			t.Fatalf("%q %v", got, err)
		}
	})
}

func TestSetupAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		status                   int
		key                      string
		interactive, environment bool
		wantLogin                bool
		wantError                string
	}{
		{name: "valid stored key", status: 200, key: "stored"},
		{name: "missing credential opens browser", interactive: true, wantLogin: true},
		{name: "expired credential opens browser", status: 401, key: "expired", interactive: true, wantLogin: true},
		{name: "missing credential in script", wantError: "interactive terminal"},
		{name: "bad environment key", status: 401, key: "bad", interactive: true, environment: true, wantError: "SKILLGILD_API_KEY"},
		{name: "server failure preserves credential", status: 503, key: "stored", interactive: true, wantError: "verify"},
		{name: "forbidden is not a login failure", status: 403, key: "stored", interactive: true, wantError: "verify"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests, logins := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/me" || r.Header.Get("Authorization") != "Bearer "+tc.key {
					t.Errorf("unexpected request %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					io.WriteString(w, `{"data":{"email":"user@example.invalid"}}`)
				} else {
					io.WriteString(w, `{"error":{"code":"error","message":"test error"}}`)
				}
			}))
			defer server.Close()
			load := func() (string, error) {
				if tc.key == "" {
					return "", agentclient.ErrNoCredential
				}
				return tc.key, nil
			}
			err := ensureSetupLoginWith(context.Background(), server.URL+"/v1", tc.interactive, load, func(context.Context, string) error { logins++; return nil }, tc.environment)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("want %q, got %v", tc.wantError, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if (logins == 1) != tc.wantLogin {
				t.Fatalf("login calls %d", logins)
			}
			if tc.key == "" && requests != 0 {
				t.Fatal("sent unauthenticated verification")
			}
		})
	}
}

func TestSetupStopsBeforeRegistrationOnLoginFailure(t *testing.T) {
	s := testOnboarding(t)
	s.authenticate = func(context.Context, string, bool) error { return errors.New("denied") }
	s.register = func(context.Context, string) error { t.Fatal("registered after failed sign-in"); return nil }
	if err := s.prepare(context.Background(), "api", "cursor"); err == nil {
		t.Fatal("missing error")
	}
	if _, err := os.Stat(s.preferencePath); !os.IsNotExist(err) {
		t.Fatal("remembered failed setup")
	}
}

func TestInstallCompletesSetupBeforeWritingWrapper(t *testing.T) {
	t.Setenv(agentclient.EnvAPIKey, "sg_test_install")
	s := testOnboarding(t, "claude")
	var calls []string
	s.authenticate = func(context.Context, string, bool) error { calls = append(calls, "login"); return nil }
	s.register = func(context.Context, string) error {
		calls = append(calls, "register")
		if _, err := os.Stat(filepath.Join(s.home, ".claude", "skills", "demo", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatal("installed before registration")
		}
		return nil
	}
	var reported string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/v1/skills/demo" {
			io.WriteString(w, `{"data":{"id":"demo-id","slug":"demo","name":"Demo","runtime_type":"hybrid_tools","distribution_mode":"hosted_private"}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/install") || strings.HasSuffix(r.URL.Path, "/installs") {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			reported, _ = body["agent"].(string)
			io.WriteString(w, `{"data":{}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	if err := installWithSetup(context.Background(), server.URL+"/v1", []string{"demo"}, s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"login", "register"}) {
		t.Fatal(calls)
	}
	raw, err := os.ReadFile(filepath.Join(s.home, ".claude", "skills", "demo", "SKILL.md"))
	if err != nil || !bytes.Contains(raw, []byte("name: demo")) {
		t.Fatalf("wrapper: %v", err)
	}
	if reported != "claude-code" {
		t.Fatalf("install reported agent %q", reported)
	}
}

func TestInstallFailureDoesNotWriteWrapper(t *testing.T) {
	s := testOnboarding(t, "claude")
	s.register = func(context.Context, string) error { return errors.New("config is malformed") }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"id":"demo","slug":"demo","name":"Demo"}}`)
	}))
	defer server.Close()
	if err := installWithSetup(context.Background(), server.URL, []string{"demo"}, s); err == nil {
		t.Fatal("install succeeded")
	}
	if _, err := os.Stat(filepath.Join(s.home, ".claude", "skills", "demo")); !os.IsNotExist(err) {
		t.Fatal("wrote wrapper despite failed registration")
	}
}

func TestInstallFilesOnlySkipsOnboarding(t *testing.T) {
	t.Setenv(agentclient.EnvAPIKey, "sg_test_install")
	s := testOnboarding(t)
	s.authenticate = func(context.Context, string, bool) error { t.Fatal("files-only install signed in"); return nil }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"id":"demo","slug":"demo","name":"Demo"}}`)
	}))
	defer server.Close()
	target := t.TempDir()
	if err := installWithSetup(context.Background(), server.URL, []string{"demo", "--target", target}, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "demo", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCommunityInstallSkipsOnboarding(t *testing.T) {
	t.Setenv(agentclient.EnvAPIKey, "sg_test_install")
	s := testOnboarding(t)
	s.authenticate = func(context.Context, string, bool) error { t.Fatal("community install signed in"); return nil }
	// An unusable source fails after the distribution decision, without signing in
	// or attempting any agent registration.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"id":"demo","slug":"demo","name":"Demo","distribution_mode":"open_source","source_url":"invalid"}}`)
	}))
	defer server.Close()
	err := installWithSetup(context.Background(), server.URL, []string{"demo", "--target", t.TempDir()}, s)
	if err == nil || !strings.Contains(err.Error(), "https source") {
		t.Fatalf("%v", err)
	}
}

func TestReadAgentConfigAndMatching(t *testing.T) {
	for _, tomlConfig := range []bool{false, true} {
		t.Run(fmt.Sprint(tomlConfig), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			config := `{"mcpServers":{"other":{"command":"other"},"skillgild":{"command":"/bin/skillgild","args":["mcp"],"env":{"KEEP":"value"}}}}`
			if tomlConfig {
				config = "[mcp_servers.other]\ncommand = 'other'\n[mcp_servers.skillgild]\ncommand = '/bin/skillgild'\nargs = ['mcp']\n[mcp_servers.skillgild.env]\nKEEP = 'value'\n"
			}
			os.WriteFile(path, []byte(config), 0600)
			entry, err := readAgentMCPEntry(path, tomlConfig)
			if err != nil {
				t.Fatal(err)
			}
			if !matchingMCPEntry(entry, "/bin/skillgild", nil) {
				t.Fatalf("not recognized: %#v", entry)
			}
			if matchingMCPEntry(entry, "/different/skillgild", nil) {
				t.Fatal("different executable matched")
			}
			if matchingMCPEntry(entry, "/bin/skillgild", map[string]string{"SKILLGILD_API_URL": "http://localhost:8080/v1"}) {
				t.Fatal("different API matched")
			}
			entry["disabled"] = true
			if matchingMCPEntry(entry, "/bin/skillgild", nil) {
				t.Fatal("disabled entry matched")
			}
		})
	}
}
