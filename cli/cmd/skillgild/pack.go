package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SkillGild/skillgild/cli/internal/agentclient"
)

// A pack installs a curated, ordered set of skills together. It uses the same install
// code as `skillgild install` for each member, and adds what a set needs on top:
// sign-in and agent setup once, a tier check before anything is written, and a receipt
// of what was installed so a second run can tell an installation it made (skip, or
// update) from a folder it did not (conflict).

type packMemberState int

const (
	packMemberMissing  packMemberState = iota // no folder: install it
	packMemberCurrent                         // our installation, same files: skip
	packMemberOutdated                        // our unmodified installation of older files: update it
	packMemberConflict                        // a folder this pack did not install, or one edited since
)

// packMemberReceipt records one installed member: what it was and the hash of its
// SKILL.md as written, so an edit made afterwards is noticed.
type packMemberReceipt struct {
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
	FileHash    string `json:"file_hash"`
}

// packInstallReceipt is one destination (an agent's skills folder) of a pack.
type packInstallReceipt struct {
	// Revision is set only when every member was in place, so a partial install shows
	// as incomplete.
	Revision  string                       `json:"revision"`
	UpdatedAt string                       `json:"updated_at"`
	Members   map[string]packMemberReceipt `json:"members"`
}

type packReceipt struct {
	Pack     string                        `json:"pack"`
	Installs map[string]packInstallReceipt `json:"installs"`
}

// classifyPackMember decides what to do with one member. A folder is the pack's own,
// and safe to replace, only when a receipt exists for it and its SKILL.md still has the
// hash recorded at install time.
func classifyPackMember(dirExists bool, installedHash string, receipt *packMemberReceipt, member agentclient.PackMember) packMemberState {
	if !dirExists {
		return packMemberMissing
	}
	if receipt == nil || installedHash == "" || installedHash != receipt.FileHash {
		return packMemberConflict
	}
	if receipt.Fingerprint == member.Fingerprint() {
		return packMemberCurrent
	}
	return packMemberOutdated
}

func hashSkillFile(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func packReceiptPath(slug string) (string, error) {
	credential, err := agentclient.CredentialFilePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(credential), "packs", slug+".json"), nil
}

func loadPackReceipt(slug string) (packReceipt, error) {
	receipt := packReceipt{Pack: slug, Installs: map[string]packInstallReceipt{}}
	path, err := packReceiptPath(slug)
	if err != nil {
		return receipt, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return receipt, nil
	}
	if err != nil {
		return receipt, err
	}
	// An unreadable receipt is treated as none: every existing folder then counts as a
	// conflict and nothing is overwritten without --force.
	if json.Unmarshal(data, &receipt) != nil || receipt.Installs == nil {
		return packReceipt{Pack: slug, Installs: map[string]packInstallReceipt{}}, nil
	}
	return receipt, nil
}

func savePackReceipt(receipt packReceipt) error {
	path, err := packReceiptPath(receipt.Pack)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return writeConfigAtomically(path, append(raw, '\n'), 0o600)
}

// packDeps are the parts of a pack install that touch the network or the terminal, so
// tests can replace them.
type packDeps struct {
	onboarding  func() (*onboarding, error)
	eligibility func(ctx context.Context, baseURL, slug string) (agentclient.PackEligibility, error)
	install     func(ctx context.Context, baseURL string, args []string) error
}

func defaultPackDeps() packDeps {
	return packDeps{
		onboarding: newOnboarding,
		eligibility: func(ctx context.Context, baseURL, slug string) (agentclient.PackEligibility, error) {
			api, err := newClient(baseURL, true)
			if err != nil {
				return agentclient.PackEligibility{}, err
			}
			return api.PackEligibility(ctx, slug)
		},
		install: func(ctx context.Context, baseURL string, args []string) error {
			return installWithSetup(ctx, baseURL, args, nil)
		},
	}
}

func pack(ctx context.Context, baseURL string, args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return fmt.Errorf("usage: skillgild pack install <pack-slug> [--agent shared|codex|cursor|claude-code|gemini-cli|antigravity|antigravity-cli] [--target path] [--force] [--no-setup]")
	}
	return packInstallWith(ctx, baseURL, args[1:], defaultPackDeps())
}

func packLookupError(slug string, err error) error {
	var apiErr *agentclient.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Status == 404:
			return fmt.Errorf("there is no published pack named %q; see https://skillgild.dev/collections for the available packs", slug)
		case apiErr.Code == "pack_unavailable":
			return fmt.Errorf("the pack %s cannot be installed right now, and nothing was changed: %s", slug, strings.TrimPrefix(apiErr.Message, "pack unavailable: "))
		}
	}
	return err
}

func packInstallWith(ctx context.Context, baseURL string, args []string, deps packDeps) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("provide a pack slug: skillgild pack install <pack-slug>")
	}
	slug := args[0]
	if !safeSkillSlug.MatchString(slug) {
		return fmt.Errorf("%q is not a valid pack slug", slug)
	}
	flags := flag.NewFlagSet("pack install", flag.ContinueOnError)
	agent := flags.String("agent", "auto", "agent preset (automatically detected for hosted skills)")
	target := flags.String("target", "", "custom skill root directory")
	force := flags.Bool("force", false, "replace skills that are already installed")
	noSetup := flags.Bool("no-setup", false, "install files only; keep login and agent configuration unchanged")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected pack arguments: %s", strings.Join(flags.Args(), " "))
	}

	api, err := newClient(baseURL, false)
	if err != nil {
		return err
	}
	pack, err := api.GetPack(ctx, slug)
	if err != nil {
		return packLookupError(slug, err)
	}
	hosted := false
	for _, member := range pack.Members {
		if !safeSkillSlug.MatchString(member.Slug) {
			return fmt.Errorf("the %s pack lists a skill with an invalid install slug; nothing was installed", slug)
		}
		hosted = hosted || member.DistributionMode != agentclient.DistributionOpenSource
	}
	automatic := hosted && *target == "" && !*noSetup && *agent != "shared"

	var flow *onboarding
	if automatic || pack.Tier == "pro" {
		if flow, err = deps.onboarding(); err != nil {
			return err
		}
	}
	// A Pro pack is checked before anything is written: sign in first (a Pro customer who
	// is signed out gets the chance to), then ask the server whether the plan is active.
	if pack.Tier == "pro" {
		if err := flow.authenticate(ctx, baseURL, flow.interactive); err != nil {
			return err
		}
		access, err := deps.eligibility(ctx, baseURL, slug)
		if err != nil {
			return packLookupError(slug, err)
		}
		if !access.Eligible {
			upgrade := access.UpgradeURL
			if upgrade == "" {
				upgrade = "https://skillgild.dev/pricing"
			}
			return fmt.Errorf("%s is a Pro pack and this account has no active SkillGild Pro plan, so nothing was installed.\nUpgrade at %s, then run the same command again. The free skills in it can still be installed one at a time with `skillgild install <skill>`", pack.Name, upgrade)
		}
	}

	if automatic {
		if *agent != "auto" {
			if _, ok := agentSkillRoot(flow.home, *agent); !ok {
				return fmt.Errorf("unsupported agent preset %q: %w", *agent, errNeedsNewer)
			}
		}
		selected, err := flow.selectAgent(*agent)
		if err != nil {
			return err
		}
		*agent = selected
	} else if *agent == "auto" {
		*agent = "shared"
	}
	root := *target
	if root == "" {
		home := ""
		if flow != nil && automatic {
			home = flow.home
		} else if home, err = os.UserHomeDir(); err != nil {
			return fmt.Errorf("find home directory")
		}
		var ok bool
		if root, ok = agentSkillRoot(home, *agent); !ok {
			return fmt.Errorf("unsupported agent preset %q; use shared, codex, cursor, claude-code, gemini-cli, antigravity, antigravity-cli, or --target: %w", *agent, errNeedsNewer)
		}
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("skill target must be an absolute path")
	}
	// Sign-in and agent registration happen once for the whole pack.
	if automatic {
		if err := flow.prepare(ctx, baseURL, *agent); err != nil {
			return err
		}
	}

	receipt, err := loadPackReceipt(slug)
	if err != nil {
		return fmt.Errorf("read the record of earlier installs of %s: %w", slug, err)
	}
	record := receipt.Installs[root]
	if record.Members == nil {
		record.Members = map[string]packMemberReceipt{}
	}

	fmt.Printf("Installing %s (%d skills) into %s\n", pack.Name, len(pack.Members), root)
	var installed, updated, current, conflicts, failed int
	for _, member := range pack.Members {
		dir := filepath.Join(root, member.Slug)
		_, statErr := os.Lstat(dir)
		var prior *packMemberReceipt
		if r, ok := record.Members[member.Slug]; ok {
			prior = &r
		}
		state := classifyPackMember(statErr == nil, hashSkillFile(dir), prior, member)
		switch {
		case state == packMemberCurrent && !*force:
			fmt.Printf("  up to date  %s\n", member.Slug)
			current++
			continue
		case state == packMemberConflict && !*force:
			fmt.Printf("  conflict    %s: %s already exists and was not installed by this pack, or was edited since\n", member.Slug, dir)
			conflicts++
			continue
		}
		installArgs := []string{member.Slug, "--agent", *agent, "--target", root, "--no-setup"}
		if state != packMemberMissing {
			installArgs = append(installArgs, "--force")
		}
		if err := deps.install(ctx, baseURL, installArgs); err != nil {
			fmt.Printf("  failed      %s: %v\n", member.Slug, err)
			failed++
			continue
		}
		record.Members[member.Slug] = packMemberReceipt{Version: member.Version, Fingerprint: member.Fingerprint(), FileHash: hashSkillFile(dir)}
		receipt.Installs[root] = record
		if err := savePackReceipt(receipt); err != nil {
			fmt.Fprintf(os.Stderr, "skillgild: installed %s but could not save the install record: %v\n", member.Slug, err)
		}
		if state == packMemberMissing {
			installed++
			fmt.Printf("  installed   %s\n", member.Slug)
		} else {
			updated++
			fmt.Printf("  updated     %s\n", member.Slug)
		}
	}

	// Skills an older revision installed and this one no longer lists are left alone.
	var dropped []string
	listed := map[string]bool{}
	for _, member := range pack.Members {
		listed[member.Slug] = true
	}
	for name := range record.Members {
		if !listed[name] {
			dropped = append(dropped, name)
		}
	}
	sort.Strings(dropped)
	if len(dropped) > 0 {
		fmt.Printf("No longer part of this pack, and left installed: %s\n", strings.Join(dropped, ", "))
	}

	if failed == 0 && conflicts == 0 {
		record.Revision = pack.Revision
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		receipt.Installs[root] = record
		if err := savePackReceipt(receipt); err != nil {
			fmt.Fprintf(os.Stderr, "skillgild: could not save the install record: %v\n", err)
		}
		fmt.Printf("%s is installed (revision %s): %d new, %d updated, %d already up to date.\n", pack.Name, pack.Revision, installed, updated, current)
		for _, member := range pack.Members {
			if member.Starter {
				fmt.Printf("Start with %s: open your agent and ask it to use %s.\n", member.Name, member.Name)
			}
		}
		return nil
	}
	if conflicts > 0 {
		fmt.Println("Folders marked conflict were left untouched. Run again with --force to replace them with the pack's versions; any changes you made to those folders will be lost.")
	}
	return fmt.Errorf("%d of %d skills in %s need attention (%d failed, %d in conflict); fix the cause and run the same command again to finish", failed+conflicts, len(pack.Members), pack.Name, failed, conflicts)
}
