package report

import "os/exec"

func runSilent(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
