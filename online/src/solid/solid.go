package solid

import (
    "encoding/binary"
    "bytes"
    "data"
    //"github.com/tarm/goserial"
    "os/exec"
    "path/filepath"
    "fmt"
    "hw"
    "io"
    "ipbus"
    "net"
    "os"
    "strconv"
    "strings"
    "time"
)

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

func (r *Reader) Run() {
    r.read = make(chan data.ReqResp, 100)
    running := true
    for running {
        // send a request to read MPPC data buffer then read remaining length
        p := ipbus.MakePacket(ipbus.Control)
        p.Add(ipbus.MakeRead(uint8(255), uint32(0x33557799)))
        p.Add(ipbus.MakeRead(uint8(108), uint32(0x33557799)))
        p.Add(ipbus.MakeRead(uint8(1), uint32(0xffeeeedd)))
        r.hw.Send(p, r.read)
        select {
        // Signal to stop
        case <- r.Stop:
            running = false
            break
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon number of words ready to read
        case data := <-r.read:
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
            //time.Sleep(r.dt)
            //time.Sleep(time.Second)
        }
    }
}

func convert(hash string) ([]byte, error) {
    val := make([]byte, 0, len(hash) / 2)
    if len(hash) % 2 != 0 {
        return val, fmt.Errorf("Odd number of chars, not sha1 hash.")
    }
    for i := 0; i < len(hash) / 2; i++ {
        n := 2 * i
        s := hash[n:n + 2]
        b, err := strconv.ParseUint(s, 16, 8)
        if err != nil {
            return val, err
        }
        if b > 255 {
            return val, fmt.Errorf("Invalid %dth byte: %d in hash %s", i, b, hash)
        }
        val = append(val, uint8(b))
    }
    return val, nil
}

type commit struct {
    hash []byte
    modified bool
}

func (c commit) String() string {
    s := fmt.Sprintf("%x", c.hash)
    if c.modified {
        s += " Modified"
    }
    return s
}

func getcommit() (commit, error) {
    c := commit{}
    cmd := exec.Command("git", "log", "-n", "1")
    out, err := cmd.Output()
    if err != nil {
        return c, err
    }
    fmt.Printf("%s\n", out)
    invalidlog := fmt.Errorf("Invalid git log: %s", out)
    commitlines := strings.Split(string(out), "\n")
    if len(commitlines) < 1 {
        return c, invalidlog
    }
    commitline := strings.Split(commitlines[0], " ")
    if commitline[0] != "commit" {
        return c, invalidlog
    }
    hash, err := convert(commitline[1])
    if err != nil {
        return c, invalidlog
    }
    c.hash = hash
    cmd = exec.Command("git", "diff")
    out, err = cmd.Output()
    if err != nil {
        return c, err
    }
    fmt.Printf("%s\n", out)
    if len(out) > 0 {
        c.modified = true
    }
    return c, error(nil)
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
func (w Writer) Run() {
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
                if err := w.outp.Close(); err != nil {
                    panic(err)
                }
                w.open = false
                if err := w.create(run); err != nil {
                    panic(err)
                }
            case <-w.Quit:
                running = false
                if err := w.outp.Close(); err != nil {
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
    c, err := getcommit()
    if err != nil {
        return err
    }
    if c.modified {
        return fmt.Errorf("Code not commited: %v", c)
    }
    header = append(header, c.hash...)
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
    c.errs = make(chan error, 100)
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
    c.w = NewWriter(c.datatowriter, c.runtowriter, c.outpdir)
    go c.w.Run()
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

    c.started = true
    return error(nil)
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
    for i, _ := range c.hws {
        rep := <-replies
        fmt.Printf("Received %dth response: %v\n", i, rep)
        c.datatowriter <- rep
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
    for i, _ := range c.hws {
        rep := <-replies
        fmt.Printf("Received %dth response: %v\n", i, rep)
        c.datatowriter <- rep
    }
    fmt.Println("Finished stopacquisition()")
}

// Start and stop a run
func (c Control) Run(name string, dt time.Duration) error {
    tick := time.NewTicker(dt)
    r := data.Run{0, name, time.Now(), time.Now().Add(dt)}
    // Tell the writer to start a new file
    c.runtowriter <- r
    // Start the readers going
    for _, reader := range c.readers {
        go reader.Run()
    }
    // Tell the FPGAs to start acquisition
    c.startacquisition()
    fmt.Printf("Run control waiting for %v.\n", dt)
    <-tick.C
    // Stop the FPGAs
    c.stopacquisition()
    // stop the readers
    for _, r := range c.readers {
        r.Stop <- true
    }
    tick.Stop()
    return error(nil)
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
