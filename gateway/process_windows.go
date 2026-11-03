package gateway

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
func stopProcess(cmd *exec.Cmd, done <-chan error) error {
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	default:
	}
	err := cmd.Process.Kill()
	<-done
	return err
}
