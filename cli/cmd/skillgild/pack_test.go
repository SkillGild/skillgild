package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

// packServer answers /packs/<slug> with the given manifest, or the given error status.
func packServer(t *testing.T, manifest *agentclient.Pack, status int, code string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": "pack unavailable: logo is no longer published"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": manifest})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func testPack(tier string, versions ...string) *agentclient.Pack {
	pack := &agentclient.Pack{Slug: "launch", Name: "Launch", Tier: tier, Revision: "rev1", Starter: "starter"}
	for i, slug := range []string{"starter", "logo", "frontend"} {
		version := "1.0.0"
		if i < len(versions) {
			version = versions[i]
		}
		pack.Members = append(pack.Members, agentclient.PackMember{Position: i, Slug: slug, Name: strings.ToUpper(slug), Version: version, DistributionMode: agentclient.DistributionOpenSource, AccessTier: "free", Starter: slug == "starter"})
	}
	return pack
}

type installRecorder struct {
	calls  [][]string
	failOn map[string]bool
	body   string
}

func (r *installRecorder) install(_ context.Context, _ string, args []string, _ agentclient.PackMember) error {
	r.calls = append(r.calls, args)
	slug := args[0]
	if r.failOn[slug] {
		return fmt.Errorf("network went away")
	}
	var root string
	for i, a := range args {
		if a == "--target" {
			root = args[i+1]
		}
	}
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(r.body+slug), 0o644)
}

func (r *installRecorder) slugs() []string {
	var out []string
	for _, c := range r.calls {
		out = append(out, c[0])
	}
	return out
}

func (r *installRecorder) forced(slug string) bool {
	for _, c := range r.calls {
		if c[0] == slug {
			for _, a := range c {
				if a == "--force" {
					return true
				}
			}
		}
	}
	return false
}

func packEnv(t *testing.T) (root string) {
	t.Helper()
	t.Setenv(agentclient.EnvConfigDir, t.TempDir())
	return filepath.Join(t.TempDir(), "skills")
}

func runPack(t *testing.T, base, root string, rec *installRecorder, extra ...string) error {
	t.Helper()
	deps := packDeps{
		onboarding: func() (*onboarding, error) {
			return nil, fmt.Errorf("onboarding must not run with --target and open-source members")
		},
		eligibility: func(context.Context, string, string) (agentclient.PackEligibility, error) {
			return agentclient.PackEligibility{Eligible: true}, nil
		},
		install: rec.install,
	}
	return packInstallWith(context.Background(), base, append([]string{"launch", "--target", root, "--agent", "shared"}, extra...), deps)
}

func TestPackInstallsEveryMemberThenSkipsWhatIsCurrent(t *testing.T) {
	root := packEnv(t)
	base := packServer(t, testPack("free"), 0, "")
	rec := &installRecorder{body: "v1 "}
	if err := runPack(t, base, root, rec); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.slugs(), ","); got != "starter,logo,frontend" {
		t.Fatalf("members must install in pack order, got %s", got)
	}
	before := len(rec.calls)
	if err := runPack(t, base, root, rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != before {
		t.Fatalf("a second run reinstalled verified skills: %v", rec.slugs()[before:])
	}
}

func TestPackNeverOverwritesAFolderItDidNotInstall(t *testing.T) {
	root := packEnv(t)
	own := filepath.Join(root, "logo")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("my own logo skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := packServer(t, testPack("free"), 0, "")
	rec := &installRecorder{body: "v1 "}
	err := runPack(t, base, root, rec)
	if err == nil || !strings.Contains(err.Error(), "need attention") {
		t.Fatalf("a conflict must fail the command so it is not mistaken for success, got %v", err)
	}
	if strings.Contains(strings.Join(rec.slugs(), ","), "logo") {
		t.Fatal("the conflicting folder was installed over")
	}
	if got, _ := os.ReadFile(filepath.Join(own, "SKILL.md")); string(got) != "my own logo skill" {
		t.Fatalf("the user's folder was modified: %q", got)
	}
	if got := strings.Join(rec.slugs(), ","); got != "starter,frontend" {
		t.Fatalf("the rest of the pack must still install, got %s", got)
	}
	// --force is the explicit request to replace it.
	if err := runPack(t, base, root, rec, "--force"); err != nil {
		t.Fatal(err)
	}
	if !rec.forced("logo") {
		t.Fatal("--force did not replace the conflicting folder")
	}
}

func TestPackUpdatesOnlyUnmodifiedInstallsOfOlderFiles(t *testing.T) {
	root := packEnv(t)
	rec := &installRecorder{body: "v1 "}
	if err := runPack(t, packServer(t, testPack("free"), 0, ""), root, rec); err != nil {
		t.Fatal(err)
	}
	// logo gets a new version upstream, and the user edits frontend.
	if err := os.WriteFile(filepath.Join(root, "frontend", "SKILL.md"), []byte("edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec.calls, rec.body = nil, "v2 "
	err := runPack(t, packServer(t, testPack("free", "1.0.0", "2.0.0", "2.0.0"), 0, ""), root, rec)
	if err == nil {
		t.Fatal("an edited folder must be reported as a conflict")
	}
	if got := strings.Join(rec.slugs(), ","); got != "logo" || !rec.forced("logo") {
		t.Fatalf("expected only logo to be updated (with --force), got %s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "frontend", "SKILL.md")); string(got) != "edited by the user" {
		t.Fatalf("the user's edit was lost: %q", got)
	}
}

func TestPackPreservesEditedSupportingFilesOnUpdate(t *testing.T) {
	root := packEnv(t)
	rec := &installRecorder{body: "v1 "}
	deps := defaultPackDeps()
	deps.install = func(ctx context.Context, base string, args []string, member agentclient.PackMember) error {
		if err := rec.install(ctx, base, args, member); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, member.Slug, "helper.py"), []byte("original"), 0o644)
	}
	if err := packInstallWith(context.Background(), packServer(t, testPack("free"), 0, ""), []string{"launch", "--target", root}, deps); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(root, "logo", "helper.py")
	if err := os.WriteFile(helper, []byte("my edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec.calls = nil
	err := packInstallWith(context.Background(), packServer(t, testPack("free", "1.0.0", "2.0.0"), 0, ""), []string{"launch", "--target", root}, deps)
	if err == nil {
		t.Fatal("an edited supporting file must block replacement")
	}
	if got, _ := os.ReadFile(helper); string(got) != "my edits" {
		t.Fatalf("supporting file overwritten: %q", got)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("unexpected reinstalls: %v", rec.slugs())
	}
	receipt, err := loadPackReceipt("launch")
	if err != nil || receipt.Installs[root].Revision != "" {
		t.Fatalf("an incomplete update retained a completed revision: %+v, %v", receipt, err)
	}
}

func TestLegacyReceiptOnlyAdoptsAnUnchangedSingleFile(t *testing.T) {
	dir := t.TempDir()
	body := []byte("original wrapper")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	legacy := packMemberReceipt{LegacyFileHash: hex.EncodeToString(sum[:])}
	if got := upgradeLegacyReceipt(dir, legacy); got.TreeHash == "" {
		t.Fatal("unchanged wrapper was not adopted")
	}
	if err := os.WriteFile(filepath.Join(dir, "helper.py"), []byte("user edits"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := upgradeLegacyReceipt(dir, legacy); got.TreeHash != "" {
		t.Fatal("supporting files cannot be verified by a legacy receipt")
	}
	if err := os.Remove(filepath.Join(dir, "helper.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("edited wrapper"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := upgradeLegacyReceipt(dir, legacy); got.TreeHash != "" {
		t.Fatal("edited wrapper was adopted")
	}
}

func TestPackRejectsChangedMemberBeforeWriting(t *testing.T) {
	root := packEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": agentclient.Skill{Slug: "logo", CurrentVersion: "2.0.0", DistributionMode: "hosted_public", AccessTier: "free"}})
	}))
	defer srv.Close()
	expected := &agentclient.PackMember{Slug: "logo", Version: "1.0.0", DistributionMode: "hosted_public", AccessTier: "free"}
	err := installSkill(context.Background(), srv.URL, []string{"logo", "--target", root}, nil, expected)
	if err == nil || !strings.Contains(err.Error(), "changed since the pack was read") {
		t.Fatalf("member drift was not rejected: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("target was created before validation: %v", err)
	}
}

func TestPackResumesAfterAPartialFailure(t *testing.T) {
	root := packEnv(t)
	base := packServer(t, testPack("free"), 0, "")
	rec := &installRecorder{body: "v1 ", failOn: map[string]bool{"logo": true}}
	if err := runPack(t, base, root, rec); err == nil {
		t.Fatal("a failed member must fail the command")
	}
	if got := strings.Join(rec.slugs(), ","); got != "starter,logo,frontend" {
		t.Fatalf("one failure must not stop the other members, got %s", got)
	}
	receipt, _ := loadPackReceipt("launch")
	if receipt.Installs[root].Revision != "" {
		t.Fatal("a partial install was recorded as complete")
	}
	rec.calls, rec.failOn = nil, nil
	if err := runPack(t, base, root, rec); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.slugs(), ","); got != "logo" {
		t.Fatalf("the retry must install only what is missing, got %s", got)
	}
}

func TestProPackIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	root := packEnv(t)
	base := packServer(t, testPack("pro"), 0, "")
	rec := &installRecorder{}
	authenticated := false
	deps := packDeps{
		onboarding: func() (*onboarding, error) {
			return &onboarding{authenticate: func(context.Context, string, bool) error { authenticated = true; return nil }}, nil
		},
		eligibility: func(context.Context, string, string) (agentclient.PackEligibility, error) {
			return agentclient.PackEligibility{Eligible: false, Tier: "pro", Reason: "pro_required"}, nil
		},
		install: rec.install,
	}
	err := packInstallWith(context.Background(), base, []string{"launch", "--target", root, "--agent", "shared"}, deps)
	if err == nil || !strings.Contains(err.Error(), "Pro pack") || !strings.Contains(err.Error(), "nothing was installed") {
		t.Fatalf("expected an upgrade message, got %v", err)
	}
	if !authenticated {
		t.Fatal("a signed-out Pro customer must get the chance to sign in before being refused")
	}
	if len(rec.calls) != 0 {
		t.Fatalf("a refused pack installed %v", rec.slugs())
	}
	if _, statErr := os.Stat(root); statErr == nil {
		t.Fatal("a refused pack created the skills folder")
	}
}

func TestUnavailablePackInstallsNothing(t *testing.T) {
	root := packEnv(t)
	base := packServer(t, nil, http.StatusConflict, "pack_unavailable")
	rec := &installRecorder{}
	err := runPack(t, base, root, rec)
	if err == nil || !strings.Contains(err.Error(), "cannot be installed right now") || !strings.Contains(err.Error(), "logo is no longer published") {
		t.Fatalf("got %v", err)
	}
	if len(rec.calls) != 0 {
		t.Fatal("a pack with a withdrawn member must not install a smaller workflow")
	}
}

func TestClassifyPackMember(t *testing.T) {
	member := agentclient.PackMember{Version: "2.0.0", MirrorSHA256: "aa"}
	current := &packMemberReceipt{Fingerprint: member.Fingerprint(), TreeHash: "h"}
	older := &packMemberReceipt{Fingerprint: "1.0.0|aa", TreeHash: "h"}
	cases := []struct {
		name    string
		exists  bool
		hash    string
		receipt *packMemberReceipt
		want    packMemberState
	}{
		{"no folder", false, "", nil, packMemberMissing},
		{"no folder but a receipt", false, "", current, packMemberMissing},
		{"matching install", true, "h", current, packMemberCurrent},
		{"older unmodified install", true, "h", older, packMemberOutdated},
		{"edited since install", true, "other", current, packMemberConflict},
		{"folder without a receipt", true, "h", nil, packMemberConflict},
		{"folder without SKILL.md", true, "", current, packMemberConflict},
	}
	for _, tc := range cases {
		if got := classifyPackMember(tc.exists, tc.hash, tc.receipt, member); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPlaceSkillKeepsTheOldFolderWhenTheReplacementFails(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "skill")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := placeSkill(filepath.Join(parent, "does-not-exist"), dest, true); err == nil {
		t.Fatal("expected the copy to fail")
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "SKILL.md")); string(got) != "old" {
		t.Fatalf("a failed replacement destroyed the existing skill: %q", got)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := placeSkill(src, dest, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "SKILL.md")); string(got) != "new" {
		t.Fatalf("replacement did not land: %q", got)
	}
	entries, _ := os.ReadDir(parent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".skillgild-stage-") {
			t.Fatalf("staging folder left behind: %s", e.Name())
		}
	}
}
