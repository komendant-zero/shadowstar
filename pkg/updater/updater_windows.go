//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000010 | 0x00000008, // CREATE_NEW_CONSOLE | DETACHED_PROCESS
	}
}
