package ipbus

import (
	"fmt"
	"io"
	"os/exec"
	"time"
)

func newDummy(port int) dummyHardware {
	/*
		cmdstring := "DummyHardwareUdp.exe"
		args := []string{"--version", "w", "--port"}
		args = append(args, fmt.Sprintf("%d", port))
		cmd := exec.Command(cmdstring, args)
	*/
	cmd := exec.Command("DummyHardwareUdp.exe", "--version", "2",
		"-b", "-V", "--port", fmt.Sprintf("%d", port))
	dummy := dummyHardware{cmd, false, make(chan bool)}
	return dummy
}

// DummyHardware runs the IPbus dummy hardware program written in C++.
// It should only be used for testing this package, which is why it is
// not exported. It requires a local installation of the uhal package,
// which itself requires running SLC6 or similar. This is the only
// part of the ipbus package with that requirement.
type dummyHardware struct {
	cmd     *exec.Cmd
	running bool
	Kill    chan bool
}

func (d *dummyHardware) Start() error {
	err := error(nil)
	if !d.running {
		fmt.Printf("Starting cmd: %v\n", d.cmd)
		err = d.cmd.Start()
		fmt.Printf("Dummy hardware now running.\n")
		d.running = true
	}
	return err
}

func (d dummyHardware) Run(dt time.Duration, log io.WriteCloser) {
	if d.running {
		return
	}
	pipe, err := d.cmd.StdoutPipe()
	go io.Copy(log, pipe)
	err = d.Start()
	if err != nil {
		panic(err)
	}
	stopped := make(chan error)
	timeout := time.NewTicker(dt)
	go d.wait(stopped)
	select {
	case _ = <-timeout.C:
		d.running = false
		fmt.Printf("Dummy hardware timed out.\n")
		d.Stop()
		err := <-stopped
		fmt.Printf("DummyHardware: %v\n", err)
		return
	case err := <-stopped:
		d.running = false
		fmt.Printf("DummyHardware: %v\n", err)
		return
	case _ = <-d.Kill:
		err := d.Stop()
		if err != nil {
			panic(err)
		}
		err = <-stopped
		fmt.Printf("DummyHardware: %v\n", err)
		return
	}
}

func (d dummyHardware) wait(errchan chan error) {
	err := d.cmd.Wait()
	errchan <- err
}

func (d *dummyHardware) Stop() error {
	err := error(nil)
	if d.running {
		err = d.cmd.Process.Kill()
		d.running = false
	}
	return err
}
