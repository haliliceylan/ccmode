// ccmode switches Claude Code's active auth/statusline profile by rewriting
// the "env" and "statusLine" keys in ~/.claude/settings.json, then execs
// claude in place so the new process picks up the change immediately.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

// managedKeys are the only top-level settings.json fields ccmode is allowed
// to set or remove. Everything else in the file is left untouched.
var managedKeys = []string{"env", "statusLine"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ccmode: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	profilesPath := filepath.Join(home, ".ccmode", "profiles.json")
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	backupsDir := filepath.Join(home, ".ccmode", "backups")

	profiles, err := loadProfiles(profilesPath)
	if err != nil {
		return err
	}

	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		printUsage(profiles)
		if len(os.Args) < 2 {
			os.Exit(1)
		}
		return nil
	}

	name := os.Args[1]
	extraArgs := os.Args[2:]

	profile, ok := profiles[name]
	if !ok {
		printUsage(profiles)
		return fmt.Errorf("unknown profile %q", name)
	}

	settings, err := loadSettings(settingsPath)
	if err != nil {
		return err
	}

	if err := backupSettings(settingsPath, backupsDir); err != nil {
		return err
	}

	applyProfile(settings, profile)

	if err := writeSettings(settingsPath, settings); err != nil {
		return err
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}

	argv := append([]string{claudePath}, extraArgs...)
	if err := syscall.Exec(claudePath, argv, os.Environ()); err != nil {
		return fmt.Errorf("launching claude: %w", err)
	}
	return nil // unreachable on success
}

// applyProfile copies each managed key from profile into settings and removes
// managed keys the profile does not define. Unmanaged keys are never touched.
func applyProfile(settings map[string]json.RawMessage, profile map[string]json.RawMessage) {
	for _, key := range managedKeys {
		if val, ok := profile[key]; ok {
			settings[key] = val
		} else {
			delete(settings, key)
		}
	}
}

func loadProfiles(path string) (map[string]map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no profiles file at %s (create it first)", path)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var profiles map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return profiles, nil
}

func loadSettings(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return settings, nil
}

func backupSettings(settingsPath, backupsDir string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to back up yet
		}
		return fmt.Errorf("reading %s for backup: %w", settingsPath, err)
	}

	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		return fmt.Errorf("creating backups dir: %w", err)
	}

	backupPath := filepath.Join(backupsDir, fmt.Sprintf("settings.json.%d.bak", time.Now().UnixNano()))
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("writing backup: %w", err)
	}
	return nil
}

func writeSettings(path string, settings map[string]json.RawMessage) error {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}
	out = append(out, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

func printUsage(profiles map[string]map[string]json.RawMessage) {
	fmt.Fprintln(os.Stderr, "usage: ccmode <profile> [claude args...]")
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "no profiles defined")
		return
	}
	fmt.Fprintln(os.Stderr, "available profiles:")
	for _, name := range names {
		fmt.Fprintln(os.Stderr, "  "+name)
	}
}
