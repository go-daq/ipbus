package solid

import (
    "encoding/binary"
    "bytes"
    "data"
    //"github.com/tarm/goserial"
    "path/filepath"
    "fmt"
    "glibxml"
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
    //nread := 0
    //buf := new(bytes.Buffer)
    //bufferlen := uint32(0)
    triggerpack := ipbus.MakePacket(ipbus.Control)
    ctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    ctrl.MaskedWrite("buf_rst", 1, &triggerpack)
    ctrl.MaskedWrite("buf_rst", 0, &triggerpack)
    ctrl.MaskedWrite("trig", 1, &triggerpack)
    ctrl.MaskedWrite("trig", 0, &triggerpack)
    fmt.Printf("Software trigger packet = %v\n", triggerpack)
    trigsent := false
    readchan := uint32(0)
    //samplesread := uint32(0)
    //wfsize := uint32(2048)
    fpgabuffer := r.hw.Module.Ports["chan"]
    chanselect := r.hw.Module.Registers["csr"].Words["ctrl"]
    for running {
        // send a request to read MPPC data buffer then read remaining length
        // Each read can request up to 255 words. To fit within one packet
        // the maximum is read 255, read 108, read size
        if !trigsent {
            //fmt.Println("Sending software trigger.")
            r.hw.Send(triggerpack, r.read)
            readchan = 0
            //samplesread = 0
            trigsent = true
        } else {
            // Send a channel select write then a bunch of reads
            // Need to make sure that reply fits in MTU bytes
            // Reply has 4 bytes packet header
            // 4 bytes chan select write transaction header
            // 8 bytes for chan select read
            // 256 * 4 = 1024 bytes to read 255 words
            // 250 * 4 = 1000 bytes to read 249 words
            // total = 4 + 4 + 8 + 1024 + 1000 = 2040 = MTU
            // For initial testing I'll just grab as much data as I can in 
            // a single packet.
            p := ipbus.MakePacket(ipbus.Control)
            chanselect.Write(readchan, &p)
            chanselect.Read(&p)
            fpgabuffer.Read(255, &p)
//            fpgabuffer.Read(249, &p)
            //samplesread = 255 + 249
            //fmt.Printf("Sending read for channel %d.\n", readchan)
            readchan += 1
            if readchan >= 38 {
                trigsent = false
            }
            r.hw.Send(p, r.read)
        }
        select {
        // Signal to stop
        case <- r.Stop:
            running = false
            break
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon Number of words ready to read
        case data := <-r.read:
            //fmt.Printf("Received reply: %v\n", data)
            r.towrite <- data
        }
    }
}

func (r Reader) Reset() {
    fmt.Printf("HW%d: doing reset.\n", r.hw.Num)
    pack := ipbus.MakePacket(ipbus.Control)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("soft_rst", 1, &pack)
    r.hw.Module.Registers["id"].Words["magic"].Read(&pack)
    reply := make(chan data.ReqResp)
    r.hw.Send(pack, reply)
    rr := <-reply
    magicids := r.hw.Module.Registers["id"].Words["magic"].GetReads(rr)
    if len(magicids) > 0 {
        fmt.Printf("Board is alive: magic ID = 0x%x\n", magicids[0])
    }
    r.towrite <- rr
    r.hw.ConfigDevice()
}

func (r Reader) SwapClocks() {
    fmt.Printf("HW%d: Swapping clock.\n", r.hw.Num)
    pack := ipbus.MakePacket(ipbus.Control)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("mmcm_rst", 1, &pack)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("clk_sel", 1, &pack)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("mmcm_rst", 0, &pack)
    reply := make(chan data.ReqResp)
    r.hw.Send(pack, reply)
    rr := <-reply
    r.towrite <- rr
}

func (r Reader) Align() {
    fmt.Printf("HW%d: Doing alignment.\n", )
    // Get alignment constants
    shifts:= []uint32{}
    incs := []uint32{}
    pack := ipbus.MakePacket(ipbus.Control)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("idelctrl_rst", 1, &pack)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("idelctrl_rst", 1, &pack)
    r.hw.Module.Registers["csr"].Words["ctrl"].MaskedWrite("idelctrl_rst", 0, &pack)
    reply := make(chan data.ReqResp)
    ch_inv := make([]uint32, 0, 38)
    ch_delay := make([]uint32, 0, 38)
    for i := 0; i < 38; i++ {
        ch_delay = append(ch_delay, 0xb)
        if i < 19 {
            ch_inv = append(ch_inv, 1)
        } else {
            ch_inv = append(ch_inv, 0)
        }
    }
    r.hw.Send(pack, reply)
    rr := <-reply
    r.towrite <-rr
    pack = ipbus.MakePacket(ipbus.Control)
    chctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["chan_ctrl"]
    increg := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    for ch := uint32(0); ch < 38; ch++ {
        r.hw.Module.Registers["csr"].Words["ctrl"].Write(ch, &pack)
        chctrl.MaskedWrite("invert", ch_inv[ch], &pack)
        chctrl.MaskedWrite("sync_en", 1, &pack)
        chctrl.MaskedWrite("phase", 1, &pack)
        chctrl.MaskedWrite("shift", shifts[ch], &pack)
        chctrl.MaskedWrite("src_sel", 0, &pack)
        for i := uint32(0); i < incs[ch]; i++ {
            increg.MaskedWrite("inc", 1, &pack)
            increg.MaskedWrite("inc", 0, &pack)
        }
        chctrl.MaskedWrite("sync_en", 0, &pack)
    }
    for ch := uint32(0); ch < 38; ch++ {
        chctrl.MaskedWrite("sync_en", 1, &pack)
    }
    fmt.Printf("Made alignment packet with %d transactions.\n",
               len(pack.Transactions))
    r.hw.Send(pack, reply)
    rr = <-reply
    r.towrite <-rr
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

func (c *Control) Send(nhw int, pack ipbus.Packet, rep chan data.ReqResp) {
    c.hws[nhw].Send(pack, rep)
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

func (c *Control) AddFPGA(mod glibxml.Module) {
    hw := hw.NewHW(len(c.hws), mod, time.Second, c.errs)
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
    /*
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
    */
}

func (c Control) stopacquisition() {
    /*
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
    */
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
