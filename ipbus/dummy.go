package ipbus

import (
	"fmt"
	"os/exec"
)

func newdummy(port int) dummyhardware {
	/*
		cmdstring := "DummyHardwareUdp.exe"
		args := []string{"--version", "w", "--port"}
		args = append(args, fmt.Sprintf("%d", port))
		cmd := exec.Command(cmdstring, args)
	*/
	cmd := exec.Command("DummyHardwareUdp.exe", "--version", "2",
		"--port", fmt.Sprintf("%d", port))
	dummy := dummyhardware{cmd, false}
	return dummy
}

// dummyhardware runs the IPbus dummy hardware program written in C++.
// It should only be used for testing this package, which is why it is
// not exported. It requires a local installation of the uhal package,
// which itself requires running SLC6 or similar. This is the only
// part of the ipbus package with that requirement.
type dummyhardware struct {
	cmd     *exec.Cmd
	running bool
}

func (d *dummyhardware) Start() error {
	err := error(nil)
	if !d.running {
		fmt.Printf("Starting cmd: %v\n", d.cmd)
		err = d.cmd.Start()
		fmt.Printf("Dummy hardware now running.\n")
		d.running = true
	}
	return err
}

func (d *dummyhardware) Stop() error {
	err := error(nil)
	if d.running {
		err = d.cmd.Process.Kill()
		d.running = false
	}
	return err
}
