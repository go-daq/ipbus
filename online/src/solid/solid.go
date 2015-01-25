package solid

import (
    "encoding/binary"
    "bytes"
    "config"
    "crash"
    "data"
    //"github.com/tarm/goserial"
    "path/filepath"
    "fmt"
    "glibxml"
    "hw"
    "io"
    "ipbus"
    "math"
    "net"
    "os"
    "os/signal"
    "strconv"
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

type ClockBoard struct {
    hw *hw.HW
    towrite chan data.ReqResp
}

func NewClockBoard(hw *hw.HW, towrite chan data.ReqResp) *ClockBoard {
    clk := ClockBoard{hw, towrite}
    return &clk
}

func (clk *ClockBoard) Reset() {
    fmt.Printf("Clock board resetting\n")
    magic := clk.hw.Module.Registers["id"].Words["magic"]
    info := clk.hw.Module.Registers["id"].Words["info"]
    synccsrctrl := clk.hw.Module.Registers["sync_csr"].Words["ctrl"]
    stat := clk.hw.Module.Registers["csr"].Words["stat"]
    p := ipbus.MakePacket(ipbus.Control)
    synccsrctrl.MaskedWrite("rst", 1, &p)
    synccsrctrl.MaskedWrite("rst", 0, &p)
    magic.Read(&p)
    info.Read(&p)
    stat.Read(&p)
    reply := make(chan data.ReqResp)
    clk.hw.Send(p, reply)
    rr := <-reply
    magics := magic.GetReads(rr)
    infos := info.GetReads(rr)
    if len(magics) > 0 && len(infos) > 0 {
        fmt.Printf("Clock is alive: magic = 0x%08x, info = 0x%08x\n", magics[0], infos[0])

    } else {
        fmt.Printf("Clock reset didn't get id.magic [%d] or id.read [%d] vaues.\n", len(magics), len(infos))
    }
    stats := stat.GetReads(rr)
    if len(stats) > 0 {
        lock := stat.GetMaskedReads("clk_lock", rr)[0]
        stop := stat.GetMaskedReads("clk_stop", rr)[0]
        fmt.Printf("Clock csr.stat = 0%08x, clk_lock = %d, clk_stop = %d, \n", stats[0], lock, stop)
    }
    clk.towrite <- rr
}

func (clk *ClockBoard) RandomRate(rate float64) {
    rdiv := uint32(math.Log2(62.5e6) - math.Log2(rate) - 1.0)
    rrate := 62.5e6 / (math.Pow(2, float64(rdiv + 1)))
    fmt.Printf("Clock board setting random triggers to %f Hz [rand_div = %d, request = %f Hz]\n", rrate, rdiv, rate)
    timingcsrctrl := clk.hw.Module.Registers["sync_csr"].Words["ctrl"]
    p := ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("rand_div", rdiv, &p)
    timingcsrctrl.Read(&p)
    reply := make(chan data.ReqResp)
    clk.hw.Send(p, reply)
    rr := <-reply
    vals := timingcsrctrl.GetReads(rr)
    if len(vals) > 0 {
        fmt.Printf("clk sync_csr.ctrl = %x\n", vals)
    }
    clk.towrite <- rr
    fmt.Printf("Clock finished RandomRate()\n")
}

func (clk *ClockBoard) StartTriggers() {
    fmt.Printf("Clock board starting random triggers.\n")
    ctrl := clk.hw.Module.Registers["sync_csr"].Words["ctrl"]

    p := ipbus.MakePacket(ipbus.Control)
    ctrl.MaskedWrite("rand_en", 1, &p)
    ctrl.Read(&p)
    rep := make(chan data.ReqResp)
    clk.hw.Send(p, rep)
    rr := <-rep
    vals := ctrl.GetReads(rr)
    if len(vals) > 0 {
        fmt.Printf("clk sync_csr.ctrl = %x\n", vals)
    }
    clk.towrite <- rr
}

func (clk *ClockBoard) StopTriggers() {
    fmt.Printf("Clock board stopping random triggers.\n")
    ctrl := clk.hw.Module.Registers["sync_csr"].Words["ctrl"]
    p := ipbus.MakePacket(ipbus.Control)
    ctrl.MaskedWrite("rand_en", 0, &p)
    ctrl.Read(&p)
    rep := make(chan data.ReqResp)
    clk.hw.Send(p, rep)
    rr := <-rep
    vals := ctrl.GetReads(rr)
    if len(vals) > 0 {
        fmt.Printf("clk sync_csr.ctrl = %x\n", vals)
    }
    clk.towrite <- rr
}

func (clk *ClockBoard) SendTrigger() {
    fmt.Printf("Clock board sending a trigger.\n")
    p := ipbus.MakePacket(ipbus.Control)
    clk.hw.Module.Registers["sync_csr"].Words["ctrl"].MaskedWrite("trig", 1, &p)
    clk.hw.Module.Registers["sync_csr"].Words["ctrl"].MaskedWrite("trig", 0, &p)
    rep := make(chan data.ReqResp)
    clk.hw.Send(p, rep)
    rr := <-rep
    clk.towrite <- rr
}

// Reader polls a HW instance to read its data buffer and sends segments
// containing MPPC data to an output channel
type Reader struct{
    hw *hw.HW
    cfg config.Glib
    Stop chan bool
    towrite, read chan data.ReqResp
    period, dt time.Duration
    channels, triggerchannels []uint32
    thresholds map[uint32]uint32
    exit *crash.Exit
}

func NewReader(hw *hw.HW, cfg config.Glib, towrite chan data.ReqResp, period,
               dt time.Duration, channels []uint32,
               thresholds map[uint32]uint32, exit *crash.Exit) *Reader {
    triggerchannels := []uint32{0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89}
    r := Reader{hw: hw, cfg: cfg, towrite: towrite, period: period, dt: dt,
                channels: channels, triggerchannels: triggerchannels,
                thresholds: thresholds, exit: exit}
    r.Stop = make(chan bool)
    return &r
}

func (r *Reader) TriggerWindow(length, offset uint32) {
    fmt.Printf("Setting trigger window length = %d, offset = %d.\n", length, offset)
    reply := make(chan data.ReqResp)
    winreg := r.hw.Module.Registers["csr"].Words["window_ctrl"]
    p := ipbus.MakePacket(ipbus.Control)
    winreg.MaskedWrite("lag", offset, &p)
    winreg.MaskedWrite("size", length, &p)
    winreg.Read(&p)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}

// Initially enable all data and trigger channels
func (r *Reader) EnableReadoutChannels() {
    fmt.Printf("Enabling readout on data and trigger channels: %v, %v\n", r.channels, r.triggerchannels)
    reply := make(chan data.ReqResp)
    ctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    for _, ch := range r.channels {
        p := ipbus.MakePacket(ipbus.Control)
        fmt.Printf("Selecting channel %d\n", ch)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        //chanctrl.MaskedWrite("src_sel", 1, &p)
        ctrl.Read(&p)
        stat.Read(&p)
        chanctrl.Read(&p)
        fmt.Printf("Enabling readout.\n")
        chanctrl.MaskedWrite("ro_en", 1, &p)
        stat.Read(&p)
        r.hw.Send(p, reply)
        rr := <-reply
        ctrls := stat.GetReads(rr)
        if len(ctrls) > 0 {
            fmt.Printf("csr.stat = %x\n", ctrls)
        }
        r.towrite <- rr
    }
    for _, ch := range r.triggerchannels {
        p := ipbus.MakePacket(ipbus.Control)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("ro_en", 1, &p)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
}

func (r *Reader) StartSelfTriggers() {
    reply := make(chan data.ReqResp)
    ctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    for _, ch := range r.channels {
        p := ipbus.MakePacket(ipbus.Control)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("t_thresh", r.thresholds[ch], &p)
        chanctrl.MaskedWrite("ro_en", 1, &p)
        chanctrl.MaskedWrite("trig_en", 1, &p)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
}

func (r *Reader) StopSelfTriggers() {
    reply := make(chan data.ReqResp)
    ctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    for _, ch := range r.channels {
        p := ipbus.MakePacket(ipbus.Control)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("trig_en", 0, &p)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
}

/*
func (r *Reader) RandomTriggerRate(rate float64) {
    rdiv := uint32(math.Log2(62.5e6) - math.Log2(rate) - 1.0)
    rrate := 62.5e6 / (math.Pow(2, float64(rdiv + 1)))
    fmt.Printf("Setting random triggers to %f Hz [rand_div = %d, request = %f Hz]\n", rrate, rdiv, rate)
    timingcsrctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    p := ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("rand_div", rdiv, &p)
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}

func (r *Reader) StartRandomTriggers() {
    reply := make(chan data.ReqResp)
    p := ipbus.MakePacket(ipbus.Control)
    fmt.Printf("Starting random triggers.\n")
    timingcsrctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    stat.Read(&p)
    //timingcsrctrl.MaskedWrite("rand_div", 15, &p)
    timingcsrctrl.MaskedWrite("rand_int", 1, &p)
    timingcsrctrl.Read(&p)
    stat.Read(&p)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}

func (r *Reader) StopRandomTriggers() {
    reply := make(chan data.ReqResp)
    // Stop random triggers
    p := ipbus.MakePacket(ipbus.Control)
    fmt.Printf("Stopping random triggers.\n")
    timingcsrctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    timingcsrctrl.MaskedWrite("rand_int", 0, &p)
    timingcsrctrl.Read(&p)
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    stat.Read(&p)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}
*/

func (r *Reader) SendSoftwareTriggers(n int) {
    reply := make(chan data.ReqResp)
    p := ipbus.MakePacket(ipbus.Control)
    ctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    for i := 0; i < n; i++ {
        ctrl.MaskedWrite("trig", 1, &p)
        ctrl.MaskedWrite("trig", 0, &p)
    }
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}


func (r *Reader) StopTriggers() {
    r.StopSelfTriggers()
    //r.StopRandomTriggers()
}

func (r *Reader) Run(errs chan data.ErrPack) {
    fmt.Printf("Reader.Run() started.\n")
    defer r.exit.CleanExit("Reader.Run()")
    r.read = make(chan data.ReqResp, 100)
    running := true
    bufferlen := uint32(0)
    bufferdata := r.hw.Module.Registers["buffer"].Ports["data"]
    buffersize := r.hw.Module.Registers["buffer"].Words["count"]
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    timecontrol := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    emptybufferdelay := 5 * time.Millisecond
    nempty := 0
    ndata := 0
    for running {
        // Read up to X words of data then read size
        p := ipbus.MakePacket(ipbus.Control)
        if bufferlen > 0 {
            if bufferlen >= 255 {
                bufferdata.Read(255, &p)
                bufferlen -= 255
            }
            // Currently reducing the value a little from 108
            if bufferlen >= 100 {
                bufferdata.Read(100, &p)
            } else {
                bufferdata.Read(uint8(bufferlen), &p)
            }
        }
        if nempty % 100 == 0 {
            stat.Read(&p)
            timecontrol.Read(&p)
        }
        /*
        if nempty == 300 {
            fmt.Printf("Sending a self trigger.\n")
            timecontrol.MaskedWrite("trig", 1, &p)
            timecontrol.MaskedWrite("trig", 0, &p)
        }
        */
        buffersize.Read(&p)
        //fmt.Printf("Sending data/buffer len read packet.\n")
        r.hw.Send(p, r.read)
        select {
        // Signal to stop
        case <- r.Stop:
            running = false
            break
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon Number of words ready to read
        case data := <-r.read:
            if nempty == 300 {
                stats := stat.GetReads(data)
                if len(stats) > 0 {
                    fmt.Printf("300 empty buffer packets. csr.stat = 0x%08x\n", stats[0])
                } else {
                    fmt.Printf("300 empty buffer packets but no stat read.\n")
                }
                stats = timecontrol.GetReads(data)
                if len(stats) > 0 {
                    fmt.Printf("300 empty buffer packets. timing.csr.ctrl = 0x%08x\n", stats[0])
                } else {
                    fmt.Printf("300 empty buffer packets but no timing.csr.ctrl read.\n")
                }
                stats = timecontrol.GetReads(data)
            }
            lengths := buffersize.GetReads(data)
            n := len(lengths)
            if n > 0 {
                bufferlen = lengths[n - 1][0]
            }
            //fmt.Printf("Received reply: %v\n", data)
            if nempty < 3 || nempty % 100 == 0 {
                r.towrite <- data
            }
            if bufferlen == 0 {
                nempty += 1
/*
                if nempty == 1 {
                    fmt.Printf("1st empty after %d with data.\n", ndata)
                }
*/
                ndata = 0
                //fmt.Printf("Buffer empty, sleeping for %v\n", emptybufferdelay)
                time.Sleep(emptybufferdelay)
            } else {
                ndata += 1
/*
                if ndata == 1 {
                    fmt.Printf("1st with data [%d] after %d empty.\n", bufferlen, nempty)
                }
*/
                nempty = 0
                //fmt.Printf("lengths = %v\n", lengths)
            }
        }
    }
    fmt.Printf("Reader finished.\n")
}

func (r *Reader) ScopeModeRun(errs chan data.ErrPack) {
    defer r.exit.CleanExit("Reader.ScopeModeRun()")
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
    csrctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    csrstat := r.hw.Module.Registers["csr"].Words["stat"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    idmagic := r.hw.Module.Registers["id"].Words["magic"]
    idinfo := r.hw.Module.Registers["id"].Words["info"]
    timingcsrctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    reply := make(chan data.ReqResp)
    // Do software reset
    fmt.Printf("Doing software reset.\n")
    p := ipbus.MakePacket(ipbus.Control)
    csrctrl.MaskedWrite("soft_rst", 1, &p)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
    time.Sleep(time.Second)
    p = ipbus.MakePacket(ipbus.Control)
    idmagic.Read(&p)
    idinfo.Read(&p)
    r.hw.Send(p, reply)
    magics := idmagic.GetReads(rr)
    infos := idinfo.GetReads(rr)
    rr = <-reply
    if len(magics) > 0 && len(infos) > 0 {
        fmt.Printf("GLIB is alive: magic = 0x%x, info = 0x%x\n", magics[0], infos[0])
    }

    // If no clock lock do mmcm reset
    p = ipbus.MakePacket(ipbus.Control)
    csrstat.Read(&p)
    r.hw.Send(p, reply)
    rr = <-reply
    clk_lock := csrstat.GetMaskedReads("clk_lock", rr)[0]
    r.towrite <- rr
    if clk_lock == 0 {
        fmt.Printf("No clock lock, reseting mmcm.\n")
        p = ipbus.MakePacket(ipbus.Control)
        csrctrl.MaskedWrite("mmcm_rst", 1, &p)
        csrctrl.MaskedWrite("mmcm_rst", 0, &p)
        r.hw.Send(p, reply)
        rr = <-reply
        r.towrite <- rr
        time.Sleep(time.Second)
    }
    // Select external clock and external synchronisation
    fmt.Printf("Selecting external clock and enabling synchronisation.\n")
    p = ipbus.MakePacket(ipbus.Control)
    csrctrl.MaskedWrite("clk_sel", 1, &p)
    timingcsrctrl.MaskedWrite("sync_en", 1, &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
    // Reset timing
    fmt.Printf("Resetting timing.\n")
    p = ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("rst", 1, &p)
    timingcsrctrl.MaskedWrite("rst", 0, &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
    // Reset delays
    fmt.Printf("Resetting delay control.\n")
    p = ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("idelctrl_rst", 1, &p)
    timingcsrctrl.MaskedWrite("idelctrl_rst", 1, &p)
    timingcsrctrl.MaskedWrite("idelctrl_rst", 0, &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
    // Reset timing buffers (enable sync on all channels first)
    fmt.Printf("Resetting timing buffers.\n")
    for _, ch := range r.channels {
        p = ipbus.MakePacket(ipbus.Control)
        csrctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("sync_en", 1, &p)
        r.hw.Send(p, reply)
        rr = <-reply
        r.towrite <- rr
    }
    p = ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("buf_rst", 1, &p)
    timingcsrctrl.MaskedWrite("buf_rst", 0, &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
}

func (r Reader) Stat() {
    fmt.Printf("Getting HW%d csr.stat\n", r.hw.Num)
    csrstat := r.hw.Module.Registers["csr"].Words["stat"]
    trigstat := r.hw.Module.Registers["trig"].Words["stat"]
    pack := ipbus.MakePacket(ipbus.Control)
    csrstat.Read(&pack)
    trigstat.Read(&pack)
    reply := make(chan data.ReqResp)
    r.hw.Send(pack, reply)
    rr := <-reply
    stats := csrstat.GetReads(rr)
    if len(stats) > 0 {
        fmt.Printf("csr.stat: 0x%08x\n", stats[0])
        fmt.Printf("csr.stat.clk_lock = %d\n", csrstat.GetMaskedReads("clk_lock", rr)[0])
        fmt.Printf("csr.stat.ro_stop = %d\n", csrstat.GetMaskedReads("ro_stop", rr)[0])
    }
    stats = trigstat.GetReads(rr)
    if len(stats) > 0 {
        fmt.Printf("trig.stat: 0x%08x\n", stats[0])
    }
    r.towrite <- rr
}

func (r Reader) Align() {
    csrctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    timectrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    fmt.Printf("HW%d: aligning data channels.\n", )
    for _, ch := range r.cfg.DataChannels {
        p := ipbus.MakePacket(ipbus.Control)
        csrctrl.MaskedWrite("chan_sel", ch.Channel, &p)
        chanctrl.MaskedWrite("phase", ch.Phase, &p)
        chanctrl.MaskedWrite("shift", ch.Shift, &p)
        chanctrl.MaskedWrite("invert", ch.Invert, &p)
        chanctrl.MaskedWrite("sync_en", 1, &p)
        for i := uint32(0); i < ch.Increment; i++ {
            timectrl.MaskedWrite("inc", 1, &p)
            timectrl.MaskedWrite("inc", 0, &p)
        }
        chanctrl.MaskedWrite("sync_en", 0, &p)
        chanctrl.Read(&p)
        reply := make(chan data.ReqResp)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
}



// Write IPbus transaction data to an output file.
type Writer struct{
    outp *os.File
    open bool
    dir, store string
    towrite chan data.ReqResp
    fromcontrol chan data.Run
    Quit chan bool
    exit *crash.Exit
}

func NewWriter(towrite chan data.ReqResp, fromcontrol chan data.Run,
               outpdir, store string) *Writer {
    w := Writer{towrite: towrite, fromcontrol: fromcontrol, dir: outpdir, store: store}
    w.Quit = make(chan bool)
    return &w
}

// Write incoming data to disk and clear first four bytes of written data
func (w Writer) Run(errs chan data.ErrPack) {
    defer w.exit.CleanExit("Writer.Run()")
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
    fmt.Printf("Wrote heder to %s.\n", w.outp.Name())
    w.open = true
    return err
}

func New(dir, store string, channels []uint32, exit *crash.Exit, inttrig bool) Control {
    c := Control{outpdir: dir, store: store, channels: channels, exit: exit, internaltrigger: inttrig}
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
    clock *ClockBoard
    sc SlowControl
    w *Writer
    internaltrigger, started bool
    exit *crash.Exit
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
        // Currently fake trigger thresholds
        fakethreshold := uint32(12000)
        fmt.Printf("Using arbitrary trigger of %d.\n", fakethreshold)
        thresholds := make(map[uint32]uint32)
        for _, ch := range c.channels {
            thresholds[ch] = fakethreshold
        }
        cfg := config.Load(hw.Num)
        r := NewReader(hw, cfg, c.datatowriter, time.Second, time.Microsecond, c.channels, thresholds, c.exit)
        c.readers = append(c.readers, r)
    }

    if !c.internaltrigger {
        mod, err := glibxml.Parse("clockboard", "c_triggered.xml")
        if err != nil {
            panic(err)
        }
        clk := hw.New(0, mod, time.Second, c.exit, c.errs)
        go clk.Run()
        c.hws = append(c.hws, clk)
        c.clock = NewClockBoard(clk, c.datatowriter)
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
    name := strings.Replace(mod.Name, "GLIB", "", 1)
    num, err := strconv.ParseInt(name, 10, 32)
    if err != nil {
        panic(err)
    }
    hw := hw.New(int(num), mod, time.Second, c.exit, c.errs)
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
    // Tell the writer to start a new file
    fmt.Printf("Starting run for %v with random triggers and %v with self triggers.\n", r.RandomDuration, r.TriggeredDuration)
    c.runtowriter <- r
    time.Sleep(time.Second)
    // Tell the FPGAs to start acquisition
    c.startacquisition()
    for i, reader := range c.readers {
        fmt.Printf("Setting up %dth reader.\n", i)
        reader.Reset()
        reader.TriggerWindow(0xff, 0x7f)
        reader.EnableReadoutChannels()
        go reader.Run(c.errs)
        time.Sleep(10 * time.Microsecond)
    }
    if !c.internaltrigger {
        for _, reader := range c.readers {
            reader.Stat()
        }
        c.clock.Reset()
        fmt.Printf("Sending sync signal from clock board.\n")
        c.clock.SendTrigger()
        for _, reader := range c.readers {
            reader.Stat()
        }
    }
    time.Sleep(100 * time.Microsecond)
    if !c.internaltrigger {
        fmt.Printf("External triggers, starting from trigger board.\n")
        c.clock.RandomRate(125.5)
        c.clock.StartTriggers()
    } else {
        /*
        for i, reader := range c.readers {
            fmt.Printf("Internal triggers: Start triggers for reader %d.\n", i)
            reader.RandomTriggerRate(0.1)
            reader.StartRandomTriggers()
        }
        */
    }
    fmt.Printf("Running random triggers for %v.\n", r.RandomDuration)
    tick := time.NewTicker(r.RandomDuration)
    err := data.MakeErrPack(error(nil))
    quit := false
    select {
    case <-tick.C:
        fmt.Printf("Reducing random trigger rate due to ticker.\n")
        if c.internaltrigger {
            /*
            for _, reader := range c.readers {
                reader.RandomTriggerRate(0.1)
            }
            */
        } else {
            c.clock.RandomRate(0.01)
        }
    case err = <-c.errs:
        fmt.Printf("Control.Run() found an error.\n")
        quit = true
    case <-c.signals:
        fmt.Printf("Run stopped by ctrl-c.\n")
        quit = true
    }
    if !quit {
        fmt.Printf("Running self triggers for %v.\n", r.TriggeredDuration)
        tick = time.NewTicker(r.TriggeredDuration)
        for _, reader := range c.readers {
            reader.StartSelfTriggers()
        }
        select {
        case <-tick.C:
            fmt.Printf("Run stopped due to ticker.\n")
            for _, reader := range c.readers {
                reader.StopTriggers()
            }
            if !c.internaltrigger {
                c.clock.StopTriggers()
            }
        case err = <-c.errs:
            fmt.Printf("Control.Run() found an error.\n")
        case <-c.signals:
            fmt.Printf("Run stopped by ctrl-c.\n")
            quit = true
        }
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
