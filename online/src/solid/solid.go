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
    "strings"
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
    channels []uint32
}

func NewReader(hw *hw.HW, towrite chan data.ReqResp, period, dt time.Duration, channels []uint32) *Reader {
    r := Reader{hw: hw, towrite: towrite, period: period, dt: dt, channels: channels}
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
    readchan := 0
    samplesread := uint32(0)
    wfsize := uint32(2048)
    fpgabuffer := r.hw.Module.Ports["chan"]
    chanselect := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanselectindex := chanselect.MaskIndices["chan_sel"]
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
            // Real testing makes it seem like the 1500 byte normal UDP limit 
            // not the 2040 reported by the GLIB is enforced.
            // For initial testing I'll just grab as much data as I can in 
            // a single packet.
            p := ipbus.MakePacket(ipbus.Control)
            if samplesread == 0 {
                chanselect.MaskedWriteIndex(chanselectindex, r.channels[readchan], &p)
            }
            chanselect.Read(&p)
            toread := wfsize - samplesread
            if toread > 355 {
                fpgabuffer.Read(255, &p)
                fpgabuffer.Read(100, &p)
                samplesread += 355
            } else {
                if toread > 255 {
                    fpgabuffer.Read(255, &p)
                    toread -= 255
                    samplesread += 255
                }
                fpgabuffer.Read(uint8(toread), &p)
                samplesread += toread
            }
            //samplesread = 255 + 249
            //fmt.Printf("Sending read for channel %d.\n", readchan)
            if samplesread == wfsize {
                readchan += 1
                samplesread = 0
                if readchan >= len(r.channels) {
                    trigsent = false
                }
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
    dir, store string
    towrite chan data.ReqResp
    fromcontrol chan data.Run
    Quit chan bool
}

func NewWriter(towrite chan data.ReqResp, fromcontrol chan data.Run,
               outpdir, store string) *Writer {
    w := Writer{towrite: towrite, fromcontrol: fromcontrol, dir: outpdir, store: store}
    w.Quit = make(chan bool)
    return &w
}

// Write incoming data to disk and clear first four bytes of written data
func (w Writer) Run(errs chan data.ErrPack) {
    defer data.Clean("Writer.Run()", errs)
    defer close(w.Quit)
    tickdt := 60 * time.Second
    tick := time.NewTicker(tickdt)
    nbytes := float64(0.0)
    lastbytes := float64(0.0)
    sumlatency := time.Duration(0)
    npackets := float64(0)
    maxlatency := time.Duration(0)
    running := true
    for running {
        if w.open {
            //fmt.Printf("Waiting for packet to write.\n")
            select {
            case <-tick.C:
                averagelatency := sumlatency.Seconds() * 1000000.0 / npackets
                writespeed := (nbytes - lastbytes) * 1e-6 / tickdt.Seconds()
                fmt.Printf("Writing at %0.2f MB/s. Written %0.2f GB total. Buffer %d of %d.\n",
                           writespeed, nbytes * 1e-9, len(w.towrite), cap(w.towrite))
                ipbusspeed := (nbytes - lastbytes) * 1e-6 / sumlatency.Seconds()
                fmt.Printf("Average IPBus transport rate = %0.2f MB / s\n", ipbusspeed)
                lastbytes = nbytes
                fmt.Printf("Average latency = %0.2f us, max = %v\n", averagelatency, 
                           maxlatency)
                maxlatency = time.Duration(0)
                sumlatency = time.Duration(0)
                npackets = 0
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
                nbytes += float64(rr.RespIndex + rr.RespSize)
                latency := rr.Received.Sub(rr.Sent)
                sumlatency += latency
                npackets += 1
                if latency > maxlatency {
                    maxlatency = latency
                }
            case run := <-w.fromcontrol:
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
    fmt.Println("Writer.end()")
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
    // If there is a long term storage directory move file there.
    if (w.store != "") {
        oldname := w.outp.Name()
        _, oldfnname := filepath.Split(oldname)
        testname := filepath.Join(w.store, oldfnname)
        newname := testname
        version := 0
        info, _ := os.Stat(newname)
        for info != nil {
            part := fmt.Sprintf("_n%d.bin", version)
            newname = strings.Replace(testname, ".bin", part, 1)
            info, _ = os.Stat(newname)
            version += 1
        }
        err = os.Rename(oldname, newname)
    }
    return err
}


var headfootencodeversion = uint16(0x0)

// Create the output file and write run header.
func (w *Writer) create(r data.Run) error {
    layout := "02Jan2006_1504"
    fn := fmt.Sprintf("SM1_%s_run%d_%s.bin", r.Start.Format(layout), r.Num,
                      r.Name)
    err := error(nil)
    fn = filepath.Join(w.dir, fn)
    fmt.Printf("Writing to %s\n", fn)
    w.outp, err = os.Create(fn)
    if err != nil {
        return err
    }
    w.open = true
    /* run header:
        header/footer version [16 bits], ReqResp encoding version [16 bits]
        header size [32 bit words]
        online software commit - 160 bit sha1 hash
        run start time - 64 bit unit nanoseconds
        target run stop time - 64 bit unit nanoseconds
    */
    size := uint32(10)
    header := make([]byte, 0, size * 4)
    buf := new(bytes.Buffer)
    err = binary.Write(buf, binary.BigEndian, headfootencodeversion)
    err = binary.Write(buf, binary.BigEndian, data.ReqRespEncodeVersion)
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

func New(dir, store string, channels []uint32) Control {
    c := Control{outpdir: dir, store: store, channels: channels}
    c.config()
    c.errs = make(chan data.ErrPack, 100)
    c.signals = make(chan os.Signal)
    signal.Notify(c.signals, os.Interrupt)
    return c
}

// Control the online DAQ software
type Control struct{
    outpdir, store string
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
    channels []uint32
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
    c.datatowriter = make(chan data.ReqResp, 100000)
    c.w = NewWriter(c.datatowriter, c.runtowriter, c.outpdir, c.store)
    go c.w.Run(c.errs)
    // Set up a HW and reader for each FPGA
    fmt.Println("Setting up HW and readers.")
    for _, hw := range c.hws {
        go hw.Run()
        r := NewReader(hw, c.datatowriter, time.Second, time.Microsecond, c.channels)
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
func (c Control) Run(r data.Run) (bool, data.ErrPack) {
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
    quit := false
    select {
    case <-tick.C:
        fmt.Printf("Run stopped due to ticker.\n")
    case err = <-c.errs:
        fmt.Printf("Control.Run() found an error.\n")
    case <-c.signals:
        fmt.Printf("Run stopped by ctrl-c.\n")
        quit = true
    }
    // Stop the FPGAs
    // Really I should do this unless the error is something that would cause
    c.stopacquisition()
    // stop the readers
    for _, r := range c.readers {
        r.Stop <- true
    }
    tick.Stop()
    return quit, err
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
