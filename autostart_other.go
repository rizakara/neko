//go:build !windows

package main

// autostartSupported is false outside Windows (menu option stays disabled).
const autostartSupported = false

func isAutostartEnabled() bool { return false }

func setAutostartEnabled(enable bool) error {
	_ = enable
	return nil
}
