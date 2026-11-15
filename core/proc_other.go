//go:build !windows

package core

import "os/exec"

// hideConsole 은 Windows 외 OS 에서는 할 일이 없다.
func hideConsole(cmd *exec.Cmd) {}
