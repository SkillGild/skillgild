package agentclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/99designs/keyring"
)

// Credential storage for the CLI and the MCP server.
//
// A key is looked up in this order:
//
//  1. The SKILLGILD_API_KEY environment variable. CI jobs, containers and MCP
//     registrations that cannot reach a credential store set it explicitly.
//  2. The operating system credential store: Windows Credential Manager, the macOS
//     Keychain, or a Secret Service / KWallet daemon on a Linux desktop.
//  3. A file under the user's configuration directory that only the user can read.
//     Headless Linux, WSL, SSH and container sessions have no credential store, and
//     `skillgild login` used to fail there; they now land in this file.
//
// `skillgild status` reports which of the three is in use.
const (
	keyringService = "SkillGild"
	keyringItem    = "api-key"

	// EnvAPIKey names the environment variable that overrides the stored credential.
	EnvAPIKey = "SKILLGILD_API_KEY"
	// EnvConfigDir overrides the directory that holds the credential file.
	EnvConfigDir = "SKILLGILD_CONFIG_DIR"

	credentialFile = "credentials.json"
)

// CredentialSource says where a loaded key came from.
type CredentialSource string

const (
	SourceEnvironment CredentialSource = "the " + EnvAPIKey + " environment variable"
	SourceKeyring     CredentialSource = "the operating system credential store"
	SourceFile        CredentialSource = "a user-only credential file"
)

// ErrNoCredential is returned when no key is stored anywhere.
var ErrNoCredential = errors.New("no SkillGild credential is stored")

type storedCredential struct {
	APIKey  string    `json:"api_key"`
	SavedAt time.Time `json:"saved_at"`
}

// The kernel keyring (keyctl) is left out on purpose: it is emptied on reboot, so a
// login would silently disappear. The library's own file backend needs a password
// prompt, which an MCP server launched by an editor cannot answer.
func openKeyring() (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName:              keyringService,
		AllowedBackends:          []keyring.BackendType{keyring.WinCredBackend, keyring.KeychainBackend, keyring.SecretServiceBackend, keyring.KWalletBackend},
		KeychainTrustApplication: true,
		LibSecretCollectionName:  "login",
	})
}

// LoadAPIKey returns the stored key, or ErrNoCredential.
func LoadAPIKey() (string, error) {
	key, _, err := LoadAPIKeyWithSource()
	return key, err
}

// LoadAPIKeyWithSource returns the stored key and where it was found.
func LoadAPIKeyWithSource() (string, CredentialSource, error) {
	if key := strings.TrimSpace(os.Getenv(EnvAPIKey)); key != "" {
		return key, SourceEnvironment, nil
	}
	if key, err := loadFromKeyring(); err == nil && key != "" {
		return key, SourceKeyring, nil
	}
	if key, err := loadFromFile(); err == nil && key != "" {
		return key, SourceFile, nil
	}
	return "", "", ErrNoCredential
}

// LoadStoredAPIKey returns the key saved by `skillgild login`, ignoring the
// environment override. Login uses it to find the credential it is about to replace.
func LoadStoredAPIKey() (string, error) {
	if key, err := loadFromKeyring(); err == nil && key != "" {
		return key, nil
	}
	if key, err := loadFromFile(); err == nil && key != "" {
		return key, nil
	}
	return "", ErrNoCredential
}

// StoreAPIKey saves the key in the credential store, or in the credential file when
// no store is available, and reports which one was used.
func StoreAPIKey(secret string) (CredentialSource, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errors.New("refusing to store an empty credential")
	}
	keyringErr := storeInKeyring(secret)
	if keyringErr == nil {
		// A stale file copy would otherwise shadow a later logout from the store.
		_ = deleteFile()
		return SourceKeyring, nil
	}
	if err := storeInFile(secret); err != nil {
		return "", fmt.Errorf("credential store unavailable (%v) and the credential file could not be written: %w", keyringErr, err)
	}
	return SourceFile, nil
}

// DeleteAPIKey removes the key from every local location. Missing entries are not errors.
func DeleteAPIKey() error {
	var problems []string
	if ring, err := openKeyring(); err == nil {
		if err := ring.Remove(keyringItem); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
			problems = append(problems, "credential store: "+err.Error())
		}
	}
	if err := deleteFile(); err != nil {
		problems = append(problems, "credential file: "+err.Error())
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// CredentialFilePath is where the fallback file lives for this user.
func CredentialFilePath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv(EnvConfigDir)); dir != "" {
		return filepath.Join(dir, credentialFile), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("find a configuration directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "skillgild", credentialFile), nil
}

func loadFromKeyring() (string, error) {
	ring, err := openKeyring()
	if err != nil {
		return "", err
	}
	item, err := ring.Get(keyringItem)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(item.Data)), nil
}

func storeInKeyring(secret string) error {
	ring, err := openKeyring()
	if err != nil {
		return err
	}
	if err := ring.Set(keyring.Item{Key: keyringItem, Data: []byte(secret), Label: "SkillGild CLI access token"}); err != nil {
		return err
	}
	// Some Linux stores accept a write without a running daemon and then cannot read
	// it back; only a round trip counts as stored.
	item, err := ring.Get(keyringItem)
	if err != nil || string(item.Data) != secret {
		return fmt.Errorf("credential store did not keep the credential")
	}
	return nil
}

func loadFromFile() (string, error) {
	path, err := CredentialFilePath()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var stored storedCredential
	if err := json.Unmarshal(raw, &stored); err != nil {
		return "", fmt.Errorf("credential file %s is not valid JSON", path)
	}
	return strings.TrimSpace(stored.APIKey), nil
}

func storeInFile(secret string) error {
	path, err := CredentialFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(storedCredential{APIKey: secret, SavedAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	// Write beside the target and rename, so a crash never leaves a half-written file.
	temp, err := os.CreateTemp(filepath.Dir(path), credentialFile+".*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if err := temp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = temp.Close()
		cleanup()
		return err
	}
	if _, err := temp.Write(append(encoded, '\n')); err != nil {
		_ = temp.Close()
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func deleteFile() error {
	path, err := CredentialFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
