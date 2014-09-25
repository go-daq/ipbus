package solid

import (
    "data"
    //"github.com/tarm/goserial"
    "fmt"
    "hw"
    "io"
    "ipbus"
    "net"
    "os"
    "time"
)

// Reader polls a HW instance to read its data buffer and sends segments
// containing MPPC data to an output channel
type Reader struct{
    hw *hw.HW
    end chan bool
    towrite chan data.ReqResp
    period, dt time.Duration
}

func (r *Reader) Run() {
    running := true
    for running {
        // send a request to read MPPC data buffer then read remaining length
        p := ipbus.MakePacket(ipbus.Control)
        p.Add(ipbus.MakeRead(uint8(255), uint32(0x33557799)))
        p.Add(ipbus.MakeRead(uint8(108), uint32(0x33557799)))
        p.Add(ipbus.MakeRead(uint8(1), uint32(0xffeeeedd)))
        c := r.hw.Send(p)
        select {
        // Signal to stop
        case <- r.end:
            running = false
            break
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon number of words ready to read
        case data := <-c:
            r.towrite <- data
            /*
            sizetran := data.In.Trans[2]
            loc := sizetran.Loc
            nleft := uint32((*data.Bytes)[loc + 4])
            for i := 0; i < 3; i++ {
                nleft += uint32((*data.Bytes)[loc + 5 + i])
            }
            if nleft == 0 {
                time.Sleep(r.period)
            } else {
                time.Sleep(r.dt)
            }
            */
            time.Sleep(r.period)
        }
    }
}


// Write IPbus transaction data to an output file.
type Writer struct{
    outp *os.File
    open bool
    towrite chan data.ReqResp
    fromcontrol chan data.Run
}

// Write incoming data to disk and clear first four bytes of written data
func (w Writer) Run() {
    nbytes := 0
    target := 10
    if w.open {
        select {
        case rr := <-w.towrite:
            // Write binary to disk
            fmt.Println("Writing to disk...")
            // Set buffer length to three bytes to signal it's finished with
            nbytes += len(*rr.Bytes)
            if nbytes > target {
                fmt.Printf("Writer received %d bytes.\n", nbytes)
                for nbytes > target {
                    target *= 10
                }
            }
            (*rr.Bytes) = (*rr.Bytes)[:3]
        case run := <-w.fromcontrol:
            if err := w.outp.Close(); err != nil {
                panic(err)
            }
            w.open = false
            if err := w.create(run); err != nil {
                panic(err)
            }
        }
    } else {
        r := <-w.fromcontrol
        fmt.Printf("Starting to write file for : %v\n", r)
        if err := w.create(r); err != nil {
            panic(err)
        }
    }
}

func (w *Writer) create(r data.Run) error {
    layout := "1504_02Jan2006"
    fn := fmt.Sprintf("sm1_%d_%s_%s.bin", r.Num, r.Start.Format(layout),
                      r.Name)
    err := error(nil)
    w.outp, err = os.Create(fn)
    return err
}

func New() Control {
    c := Control{}
    c.config()
    c.errs = make(chan error, 100)
    return c
}

// Control the online DAQ software
type Control struct{
    hws []hw.HW
    packettohws []chan ipbus.Packet
    runtowriter chan data.Run
    datatowriter chan data.ReqResp
    readers []Reader
    stopreaders []chan bool
    sc SlowControl
    w Writer
    started bool
    errs chan error
}

// Prepare for the first run by configuring the FPGAs, etc
func (c *Control) Start() error {
    fmt.Println("Starting control.")
    if c.started {
        return error(nil)
    }
    // Set up the writer
    c.runtowriter = make(chan data.Run)
    c.datatowriter = make(chan data.ReqResp, 100)
    c.w = Writer{towrite: c.datatowriter, fromcontrol: c.runtowriter}
    go c.w.Run()
    // Set up a HW and reader for each FPGA
    fmt.Println("Setting up HW and readers.")
    for _, hw := range c.hws {
        go hw.Run()
        stopreader := make(chan bool)
        c.stopreaders = append(c.stopreaders, stopreader)
        r := Reader{&hw, stopreader, c.datatowriter, time.Second,
                    1 * time.Microsecond}
        c.readers = append(c.readers, r)
    }

    // Start up the slow control
    fmt.Println("Setting up slow control.")

    // Configure the FPGA
    fmt.Println("Configuring the FPGAs.")

    c.started = true
    return error(nil)
}

func (c *Control) AddFPGA(addr *net.UDPAddr) {
    tosend := make(chan ipbus.Packet, 100)
    c.packettohws = append(c.packettohws, tosend)
    hw := hw.NewHW(len(c.hws), addr, time.Second, tosend, c.errs)
    c.hws = append(c.hws, hw)
    fmt.Printf("hardwares = %v\n", c.hws)
    fmt.Printf("channels to HWs: %v\n", c.packettohws)
}

func (c *Control) connect() error {
    return error(nil)
}

// Set the zero supression and trigger thresholds in the FPGAs
func (c Control) setthresholds() error {
    return error(nil)

}

// Configure the triggers in the FPGAs
func (c Control) configtriggers() error {
    return error(nil)

}

func (c Control) config() error {
    return error(nil)
}

func (c Control) startacquisition() {
    modeaddr := uint32(0xaabbccdd)
    start := []byte{0xff, 0xff, 0xff, 0xff}
    pack := ipbus.MakePacket(ipbus.Control)
    pack.Add(ipbus.MakeWrite(modeaddr, start))
    fmt.Printf("Sending start cmd to FPGAs: %v\n", pack)
    for _, hwch := range c.packettohws {
        fmt.Printf("Sending FPGA START to %v\n", hwch)
        hwch <- pack
    }
    fmt.Println("Send START commands.")
}

func (c Control) stopacquisition() {
    modeaddr := uint32(0xaabbccdd)
    start := []byte{0x0, 0x0, 0x0, 0x0}
    pack := ipbus.MakePacket(ipbus.Control)
    pack.Add(ipbus.MakeWrite(modeaddr, start))
    fmt.Printf("Sending stop cmd to FPGAs: %v\n", pack)
    for _, hwch := range c.packettohws {
        hwch <- pack
    }
    fmt.Println("Send STOP commands.")

}

// Start and stop a run
func (c Control) Run(name string, dt time.Duration) error {
    tick := time.NewTicker(dt)
    r := data.Run{0, name, time.Now(), time.Now().Add(dt)}
    // Tell the writer to start a new file
    c.runtowriter <- r
    // Start the readers going
    /*
    for _, reader := range c.readers {
        go reader.Run()
    }
    */
    // Tell the FPGAs to start acquisition
    c.startacquisition()
    fmt.Printf("Run control waiting for %v.\n", dt)
    <-tick.C
    // Stop the FPGAs
    c.stopacquisition()
    // stop the readers
    for _, stopreader := range c.stopreaders {
        stopreader <- true
    }
    tick.Stop()
    return error(nil)
}

// Cleanly stop the online DAQ software
func (c Control) Quit() error {
    return error(nil)

}

// Control the MPPC bais voltages, read temperatures, configure ADCs
type SlowControl struct{
    Addr net.UDPAddr
    Name string
    Baud int
    ser io.ReadWriteCloser
    towriter chan data.ReqResp
    period time.Duration
    targetovervoltage, nominalvoltage []float32
    nominaltemp, tempcoefficient float32
}

// Update the voltages based upon temperature measurments
func (sc SlowControl) setvoltages() error {
    return error(nil)

}

// Configure the ADCs to read data correctly
func (sc SlowControl) configADC() error {

    return error(nil)
}
