// +build windows

package ibeamcorelib

import (
	"syscall"

	log "github.com/s00500/env_logger"
)

// Windows does not implement signaling, there fore we do not support this reload for now
const (
	SIGINT  = 1
	SIGQUIT = 2
	SIGTERM = 3
	SIGUSR2 = 4
)

// Re-exec this same image
func execReload() error {
	log.Error("Reload: execReload: unsupported on windows")
	return nil
}

// kill process specified in the environment with the signal specified in the
// environment; default to SIGQUIT.
func kill() error {
	log.Error("Reload: kill: unsupported on windows")
	return nil
}

func ReloadHook() {
	log.Debug("Reload hook, unsupported on windows")
}

func isChild() bool {
	return false
}

func wait() (syscall.Signal, error) {
	select {}
}
