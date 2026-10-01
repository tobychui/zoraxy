package shellexec

import (
	"os/exec"
	"sync"
)

/*
	Shell Exec
	author: tobychui

	Resolve a usable POSIX shell for commands that need pipes or
	other shell features. bash is not always available (e.g. the
	Alpine based docker image or a stock FreeBSD install), so this
	falls back to sh when bash cannot be found.

	All commands passed to Command must be POSIX sh compatible.
*/

var (
	dockerMode   bool
	resolveOnce  sync.Once
	resolvedPath string
)

// SetDockerMode tells the resolver Zoraxy is running inside a container,
// where bash is usually missing, so sh is tried first.
// Must be called before the first call to Command.
func SetDockerMode(enabled bool) {
	dockerMode = enabled
}

// Shell returns the path of the shell used to execute commands
func Shell() string {
	resolveOnce.Do(func() {
		candidates := []string{"bash", "sh"}
		if dockerMode {
			candidates = []string{"sh", "bash"}
		}
		for _, c := range candidates {
			if p, err := exec.LookPath(c); err == nil {
				resolvedPath = p
				return
			}
		}
		//Nothing found, let exec report the error on run
		resolvedPath = "sh"
	})
	return resolvedPath
}

// Command returns an exec.Cmd that runs script with the resolved shell
func Command(script string) *exec.Cmd {
	return exec.Command(Shell(), "-c", script)
}
