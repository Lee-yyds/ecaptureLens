//go:build !windows

package gateway

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
}
func stopProcess(cmd *exec.Cmd, done <-chan error) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	if err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		err = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			err = nil
		}
		<-done
		return err
	}
}
