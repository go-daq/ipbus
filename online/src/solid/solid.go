package solid

import (
    "data"
    "github.com/tarm/goserial"
    "io"
    "ipbus"
    "net"
    "os"
    "time"
)

// Reader polls a HW instance to read its data buffer and sends segments
// containing MPPC data to an output channel
type Reader struct{
    hw *ipbus.HW
    end chan bool
    towrite chan data.Packet
    dt time.Duration
}

func (r *Reader) Run() {
    running := true
    for {
        // send a request to read MPPC data buffer then read remaining length
        select {
        // Signal to stop
        case: <- end:
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon number of words ready to read
        case: data, ok := <-x
        }
    }
}


// Write IPbus transaction data to an output file.
type Writer struct{
    outp os.File
    towrite chan data.Packet
    fromcontrol chan data.Run
}

// Write incoming data to disk and clear first four bytes of written data
func (w Writer) Run() {

}

// Control the online DAQ software
type Control struct{
    buffers []data.Buffer
    hws []ipbus.HW
    towriter chan data.Run
    readers []Reader
    sc SlowControl
    w Writer
}

// Prepare for the first run by configuring the FPGAs, etc
func (c *Control) Start() error {

}

func (c *Control) connect() error() {

}

// Set the zero supression and trigger thresholds in the FPGAs
func (c Control) setthresholds() error {

}

// Configure the triggers in the FPGAs
func (c Control) configtriggers() error {

}

// Start a run
func (c Control) Run(name string dt time.Duration) error {

}

// Cleanly stop the online DAQ software
func (c Control) Quit() error {

}

// Control the MPPC bais voltages, read temperatures, configure ADCs
type SlowControl struct{
    Addr net.UDPAddr
    Name string
    Baud int
    ser io.ReadWriteCloser
    towriter chan data.Packet
    period time.Duration
    targetovervoltage, nominalvoltage []float32
    nominaltemp, tempcoefficient float32
}

// Update the voltages based upon temperature measurments
func (sc SlowControl) setvoltages() error {

}

// Configure the ADCs to read data correctly
func (sc SlowControl) configADC() error {

}
