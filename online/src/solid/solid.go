package solid

import (
    "encoding/binary"
    "bytes"
    "data"
    //"github.com/tarm/goserial"
    "path/filepath"
    "fmt"
    "hw"
    "io"
    "ipbus"
    "net"
    "os"
    "os/signal"
    "time"
)

// Define some register locations in the FPGAs
var regbuffersize uint32 = 0xaaaaaaaa
var regbuffer uint32 = 0xbbbbbbbb
var bregbuffersize []byte = []byte{0xaa, 0xaa, 0xaa, 0xaa}
var bregbuffer []byte = []byte{0xbb, 0xbb, 0xbb, 0xbb}

func compreg(a, b []byte) bool {
    if len(a) != len(b) {
        return false
    }
    for i, va := range a {
        if va != b[i] {
            return false
        }
    }
    return true
}

// Reader polls a HW instance to read its data buffer and sends segments
// containing MPPC data to an output channel
type Reader struct{
    hw *hw.HW
    Stop chan bool
    towrite, read chan data.ReqResp
    period, dt time.Duration
}

func NewReader(hw *hw.HW, towrite chan data.ReqResp, period, dt time.Duration) *Reader {
    r := Reader{hw: hw, towrite: towrite, period: period, dt: dt}
    r.Stop = make(chan bool)
    return &r
}

func (r *Reader) Run(errs chan data.ErrPack) {
    defer data.Clean("Reader.Run()", errs)
    r.read = make(chan data.ReqResp, 100)
    running := true
    nread := 0
    buf := new(bytes.Buffer)
    bufferlen := uint32(0)
    for running {
        // send a request to read MPPC data buffer then read remaining length
        // Each read can request up to 255 words. To fit within one packet
        // the maximum is read 255, read 108, read size
        p := ipbus.MakePacket(ipbus.Control)
        secondlimit := 255
        if nread > 0 {
            if nread > 255 {
                //fmt.Printf("Reader%d: nread = %d, adding a 255 word read request.\n", r.hw.Num, nread)
                p.Add(ipbus.MakeRead(uint8(255), regbuffer)) // read from buffer
                nread -= 255
                secondlimit = 108
            }
            if nread < secondlimit {
                //fmt.Printf("Reader%d: nread = %d, adding a %d word read request.\n", r.hw.Num, nread, nread)
                p.Add(ipbus.MakeRead(uint8(nread), regbuffer)) // read from buffer
                nread -= nread
            } else {
                //fmt.Printf("Reader%d: nread = %d, adding a %d word read request.\n", r.hw.Num, nread, secondlimit)
                p.Add(ipbus.MakeRead(uint8(secondlimit), regbuffer))
                nread -= secondlimit
            }
        }
        //fmt.Printf("Reader%d: After request nread = %d\n", r.hw.Num, nread)
        p.Add(ipbus.MakeRead(uint8(1), regbuffersize)) // Read how many words are left in buffer
        r.hw.Send(p, r.read)
        select {
        // Signal to stop
        case <- r.Stop:
            running = false
            break
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon Number of words ready to read
        case data := <-r.read:
            r.towrite <- data
            // Update the Number of words to read from the FPGA's buffer.
            // If the buffer is empty then sleep for a little time.
            loc := 4
            valloc := data.RespIndex + 4
            bufferlen = 0
            foundlen := false
            for _, t := range data.Out.Transactions {
                loc += 4
                valloc += 4
                if t.Type == ipbus.Read {
                    //fmt.Printf("Reader%d: %d: Comparing req[%d] register %x with %x\n", r.hw.Num, i, loc, data.Bytes[loc:loc + 4], bregbuffersize)
                    if compreg(data.Bytes[loc:loc + 4], bregbuffersize) {
                        //fmt.Printf("Reader%d: Found read of buffer length register\n.", r.hw.Num)
                        buf.Write(data.Bytes[valloc:valloc + 4])
                        err := binary.Read(buf, binary.BigEndian, &bufferlen)
                        if err != nil {
                            panic(err)
                        }
                        buf.Reset()
                        //fmt.Printf("Reader%d: Found buffer length = %d\n", r.hw.Num, bufferlen)
                        nread = int(bufferlen)
                        foundlen = true
                    }
                    valloc += 4 * int(t.Words)
                    loc += 4
                }
            }
            if !foundlen {
                //fmt.Printf("Reader%d: WARNING: Didn't find length of FPGA data buffer: nread = %d.\n", r.hw.Num, nread)
            }
            if nread == 0 {
                //fmt.Printf("Reader%d: FPGA buffer empty, sleeping.\n", r.hw.Num)
                time.Sleep(1000 * time.Millisecond)
            }
        }
    }
}



// Write IPbus transaction data to an output file.
type Writer struct{
    outp *os.File
    open bool
    dir string
    towrite chan data.ReqResp
    fromcontrol chan data.Run
    Quit chan bool
}

func NewWriter(towrite chan data.ReqResp, fromcontrol chan data.Run,
               outpdir string) *Writer {
    w := Writer{towrite: towrite, fromcontrol: fromcontrol, dir: outpdir}
    w.Quit = make(chan bool)
    return &w
}
// Write incoming data to disk and clear first four bytes of written data
func (w Writer) Run(errs chan data.ErrPack) {
    defer data.Clean("Writer.Run()", errs)
    defer close(w.Quit)
    nbytes := 0
    target := 10
    running := true
    start := time.Now()
    for running {
        if w.open {
            //fmt.Printf("Waiting for packet to write.\n")
            select {
            case rr := <-w.towrite:
                // Write binary to disk
                towrite, err := rr.Encode()
                if err != nil {
                    panic(err)
                }
                nwritten := 0
                ntowrite := len(towrite)
                for nwritten < ntowrite {
                    n, err := w.outp.Write(towrite[nwritten:])
                    if err != nil {
                        panic(err)
                    }
                    nwritten += n
                }
                //fmt.Println("Writing to disk...")
                nbytes += len(rr.Bytes)
                if nbytes > target {
                    fmt.Printf("Writer received %d bytes.\n", nbytes)
                    for nbytes > target {
                        target *= 10
                    }
                    dt := rr.Received.Sub(rr.Sent)
                    fmt.Printf("Latency = %v\n", dt)
                }
            case run := <-w.fromcontrol:
                w.end()
                if err := w.end(); err != nil {
                    panic(err)
                }
                if err := w.create(run); err != nil {
                    panic(err)
                }
            case <-w.Quit:
                running = false
                if err := w.end(); err != nil {
                    panic(err)
                }
                end := time.Now()
                runtime := end.Sub(start)
                rate := float64(nbytes) / runtime.Seconds() / 1000000.0
                fmt.Printf("Writer received average rate of %v MB/s\n", rate)
                fmt.Printf("%d bytes in %v.\n", nbytes, runtime)
            }
        } else {
            r := <-w.fromcontrol
            fmt.Printf("Starting to write file for : %v\n", r)
            if err := w.create(r); err != nil {
                panic(err)
            }
        }
    }
}

func (w *Writer) end() error {
    buf := new(bytes.Buffer)
    now := time.Now().UnixNano()
    if err := binary.Write(buf, binary.BigEndian, uint32(0)); err != nil {
        return err
    }
    if err := binary.Write(buf, binary.BigEndian, now); err != nil {
        return err
    }
    if _, err := w.outp.Write(buf.Bytes()); err != nil {
        return err
    }
    w.open = false
    err := w.outp.Close()
    return err
}


// Create the output file and write run header.
func (w *Writer) create(r data.Run) error {
    layout := "1504_02Jan2006"
    fn := fmt.Sprintf("SM1_%d_%s_%s.bin", r.Num, r.Start.Format(layout),
                      r.Name)
    err := error(nil)
    fn = filepath.Join(w.dir, fn)
    w.outp, err = os.Create(fn)
    if err != nil {
        return err
    }
    w.open = true
    /* run header:
        header size [32 bit words]
        online software commit - 160 bit sha1 hash
        run start time - 64 bit unit nanoseconds
        target run stop time - 64 bit unit nanoseconds
        ???
        ???
    */
    size := uint32(9)
    header := make([]byte, 0, size * 4)
    buf := new(bytes.Buffer)
    err = binary.Write(buf, binary.BigEndian, size)
    header = append(header, buf.Bytes()...)
    buf.Reset()
    header = append(header, r.Commit.Hash...)
    err = binary.Write(buf, binary.BigEndian, r.Start.UnixNano())
    if err != nil {
        return err
    }
    err = binary.Write(buf, binary.BigEndian, r.End.UnixNano())
    if err != nil {
        return err
    }
    header = append(header, buf.Bytes()...)
    nwritten := 0
    for nwritten < len(header) {
        n, err := w.outp.Write(header)
        if err != nil {
            return err
        }
        nwritten += n
    }
    return err
}

func New(dir string) Control {
    c := Control{outpdir: dir}
    c.config()
    c.errs = make(chan data.ErrPack, 100)
    c.signals = make(chan os.Signal)
    signal.Notify(c.signals, os.Interrupt)
    return c
}

// Control the online DAQ software
type Control struct{
    outpdir string
    hws []*hw.HW
    packettohws []chan ipbus.Packet
    runtowriter chan data.Run
    datatowriter chan data.ReqResp
    readers []*Reader
    sc SlowControl
    w *Writer
    started bool
    errs chan data.ErrPack
    signals chan os.Signal
}

// Prepare for the first run by configuring the FPGAs, etc
func (c *Control) Start() data.ErrPack {
    fmt.Println("Starting control.")
    if c.started {
        return data.MakeErrPack(error(nil))
    }
    // Set up the writer
    c.runtowriter = make(chan data.Run)
    c.datatowriter = make(chan data.ReqResp, 100)
    c.w = NewWriter(c.datatowriter, c.runtowriter, c.outpdir)
    go c.w.Run(c.errs)
    // Set up a HW and reader for each FPGA
    fmt.Println("Setting up HW and readers.")
    for _, hw := range c.hws {
        go hw.Run()
        r := NewReader(hw, c.datatowriter, time.Second, time.Microsecond)
        c.readers = append(c.readers, r)
    }

    // Start up the slow control
    fmt.Println("Setting up slow control.")

    // Configure the FPGA
    fmt.Println("Configuring the FPGAs.")

    //time.Sleep(time.Second)
    err := data.MakeErrPack(error(nil))
    select {
    case err = <-c.errs:
    default:
    }

    c.started = true
    return err
}

func (c *Control) AddFPGA(addr *net.UDPAddr) {
    hw := hw.NewHW(len(c.hws), addr, time.Second, c.errs)
    c.hws = append(c.hws, hw)
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
    replies := make(chan data.ReqResp)
    for _, hw := range c.hws {
        fmt.Printf("Sending FPGA START to %v\n", hw)
        hw.Send(pack, replies)
    }
    tick := time.NewTicker(5 * time.Second)
    for i, _ := range c.hws {
        select {
        case rep := <-replies:
            fmt.Printf("Received %dth response: %v\n", i, rep)
            c.datatowriter <- rep
        case <-tick.C:
            fmt.Println("startacquisition timed out.")
            break
        }
    }
    fmt.Println("Finished startacquisition()")
}

func (c Control) stopacquisition() {
    modeaddr := uint32(0xaabbccdd)
    start := []byte{0x0, 0x0, 0x0, 0x0}
    pack := ipbus.MakePacket(ipbus.Control)
    pack.Add(ipbus.MakeWrite(modeaddr, start))
    fmt.Printf("Sending stop cmd to FPGAs: %v\n", pack)
    replies := make(chan data.ReqResp)
    for _, hw := range c.hws {
        fmt.Printf("Sending FPGA START to %v\n", hw)
        hw.Send(pack, replies)
    }
    tick := time.NewTicker(5 * time.Second)
    for i, _ := range c.hws {
        select {
        case rep := <-replies:
            fmt.Printf("Received %dth response: %v\n", i, rep)
            c.datatowriter <- rep
        case <-tick.C:
            fmt.Println("stopacquisition timed out.")
            break
        }
    }
    fmt.Println("Finished stopacquisition()")
}

// Start and stop a run
func (c Control) Run(r data.Run) data.ErrPack {
    dt := r.End.Sub(time.Now())
    tick := time.NewTicker(dt)
    // Tell the writer to start a new file
    c.runtowriter <- r
    // Start the readers going
    for _, reader := range c.readers {
        go reader.Run(c.errs)
    }
    // Tell the FPGAs to start acquisition
    c.startacquisition()
    fmt.Printf("Run control waiting for %v.\n", dt)
    err := data.MakeErrPack(error(nil))
    select {
    case <-tick.C:
    case err = <-c.errs:
        fmt.Printf("Control.Run() found an error.\n")
    case <-c.signals:
        fmt.Printf("Run stopped by ctrl-c.\n")
    }
    // Stop the FPGAs
    // Really I should do this unless the error is something that would cause
    c.stopacquisition()
    // stop the readers
    for _, r := range c.readers {
        r.Stop <- true
    }
    tick.Stop()
    return err
}

// Cleanly stop the online DAQ software
func (c Control) Quit() error {
    c.w.Quit <- true
    _, ok := <-c.w.Quit
    if !ok {
        fmt.Printf("Writer quit successfully.\n")
    }
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
