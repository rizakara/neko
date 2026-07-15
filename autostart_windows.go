//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const autostartRegValue = "Psinoza"

// autostartSupported is true on platforms that can register a login run entry.
const autostartSupported = true

func autostartExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

// quotedAutostartCommand returns the Run key value for the current executable.
func quotedAutostartCommand(exe string) string {
	// Quote so paths with spaces still launch correctly.
	if strings.ContainsAny(exe, " \t") {
		return `"` + exe + `"`
	}
	return exe
}

// isAutostartEnabled reports whether Psinoza is registered for the current user.
func isAutostartEnabled() bool {
	exe, err := autostartExePath()
	if err != nil {
		return false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	val, _, err := k.GetStringValue(autostartRegValue)
	if err != nil {
		return false
	}
	// Compare unquoted paths; ignore case on Windows.
	want := strings.Trim(quotedAutostartCommand(exe), `"`)
	got := strings.Trim(val, `"`)
	return strings.EqualFold(filepath.Clean(got), filepath.Clean(want))
}

// setAutostartEnabled adds or removes the current-user Run registry entry.
func setAutostartEnabled(enable bool) error {
	exe, err := autostartExePath()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer k.Close()

	if !enable {
		// Missing value is fine (already off).
		if err := k.DeleteValue(autostartRegValue); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("remove autostart: %w", err)
		}
		return nil
	}

	if err := k.SetStringValue(autostartRegValue, quotedAutostartCommand(exe)); err != nil {
		return fmt.Errorf("set autostart: %w", err)
	}
	return nil
}
