//go:build windows

package e2e_test

import "os"

// Windows has no child-process SIGTERM equivalent; Linux CI covers graceful exit.
func terminateProcess(process *os.Process) (bool, error) {
	return false, process.Kill()
}
