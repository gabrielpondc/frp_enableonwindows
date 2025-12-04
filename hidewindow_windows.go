//go:build windows
// +build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow configures a command to run without showing a window on Windows
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
