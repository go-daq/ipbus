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
    Stop chan chan bool
    towrite, read chan data.ReqResp
    period, dt time.Duration
    channels, triggerchannels []uint32
    thresholds map[uint32]uint32
    exit *crash.Exit
}

func NewReader(hw *hw.HW, cfg config.Glib, towrite chan data.ReqResp, period,
               dt time.Duration, channels []uint32, exit *crash.Exit) *Reader {
    triggerchannels := []uint32{0x90, 0x91}
    r := Reader{hw: hw, cfg: cfg, towrite: towrite, period: period, dt: dt,
                channels: channels, triggerchannels: triggerchannels, exit: exit}
    r.Stop = make(chan chan bool)
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

func (r *Reader) DisableReadout() {
    reply := make(chan data.ReqResp)
    ctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    for _, ch := range r.channels {
        p := ipbus.MakePacket(ipbus.Control)
        fmt.Printf("Selecting channel %d\n", ch)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        ctrl.Read(&p)
        stat.Read(&p)
        chanctrl.Read(&p)
        fmt.Printf("Disabling readout.\n")
        chanctrl.MaskedWrite("ro_en", 0, &p)
        stat.Read(&p)
        r.hw.Send(p, reply)
        rr := <-reply
        ctrls := stat.GetReads(rr)
        if len(ctrls) > 0 {
            fmt.Printf("csr.stat = %x\n", ctrls)
        }
        r.towrite <- rr
    }
    /*
    for _, ch := range r.triggerchannels {
        p := ipbus.MakePacket(ipbus.Control)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("ro_en", 0, &p)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
    */

    }
// Initially enable all data and trigger channels
func (r *Reader) EnableReadoutChannels() {
    fmt.Printf("Enabling readout on data and trigger channels: %v, %v\n", r.channels, r.triggerchannels)
    reply := make(chan data.ReqResp)
    ctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    for _, ch := range r.cfg.DataChannels {
        if !ch.ReadoutEnabled {
            fmt.Printf("Readout disabled for channel %d\n", ch.Channel)
            p := ipbus.MakePacket(ipbus.Control)
            fmt.Printf("Selecting channel %d\n", ch)
            ctrl.MaskedWrite("chan_sel", ch.Channel, &p)
            //chanctrl.MaskedWrite("src_sel", 1, &p)
            ctrl.Read(&p)
            stat.Read(&p)
            chanctrl.Read(&p)
            chanctrl.MaskedWrite("ro_en", 0, &p)
            stat.Read(&p)
            r.hw.Send(p, reply)
            rr := <-reply
            ctrls := stat.GetReads(rr)
            if len(ctrls) > 0 {
                fmt.Printf("csr.stat = %x\n", ctrls)
            }
            r.towrite <- rr
        } else {
            p := ipbus.MakePacket(ipbus.Control)
            fmt.Printf("Selecting channel %d\n", ch.Channel)
            ctrl.MaskedWrite("chan_sel", ch.Channel, &p)
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
    }
    // Readout of 0x90, 0x91 readout channels is always enabled
    /* 
    for _, ch := range r.cfg.TriggerChannels {
        fmt.Printf("Enabling readout of trigger channel 0x%x\n", ch)
        p := ipbus.MakePacket(ipbus.Control)
        ctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("ro_en", 1, &p)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
    */
}

func (r *Reader) SetCoincidenceMode(mode bool) {
    if mode {
        fmt.Printf("GLIB%d enabling coincidence trigger.", r.hw.Num)
    } else {
        fmt.Printf("GLIB%d disabling coincidence trigger.", r.hw.Num)
    }
    trigctrl, ok := r.hw.Module.Registers["trig"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("Did not find trig.ctrl word."))
    }
    p := ipbus.MakePacket(ipbus.Control)
    val := uint32(0)
    if mode {
        val = 1
    }
    trigctrl.MaskedWrite("en_coinc", val, &p)
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}

func (r *Reader) StartSelfTriggers(thr uint32) {
    r.cfg.SetThresholds(thr)
    reply := make(chan data.ReqResp)
    ctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    for _, ch := range r.cfg.DataChannels {
        if !ch.TriggerEnabled {
            fmt.Printf("GLIB%d, channel %d trigger disabled.\n", r.hw.Num, ch.Channel)
            p := ipbus.MakePacket(ipbus.Control)
            ctrl.MaskedWrite("chan_sel", ch.Channel, &p)
            chanctrl.MaskedWrite("trig_en", 0, &p)
            r.hw.Send(p, reply)
            rr := <-reply
            r.towrite <- rr
        } else {
            thr, err := r.cfg.GetThreshold(ch.Channel)
            if err != nil {
                panic(err)
            }
            fmt.Printf("GLIB%d: Set channel %d threshold = %d\n", r.hw.Num, ch, thr)
            p := ipbus.MakePacket(ipbus.Control)
            ctrl.MaskedWrite("chan_sel", ch.Channel, &p)
            chanctrl.MaskedWrite("t_thresh", thr, &p)
            chanctrl.MaskedWrite("ro_en", 1, &p)
            chanctrl.MaskedWrite("trig_en", 1, &p)
            r.hw.Send(p, reply)
            rr := <-reply
            r.towrite <- rr
        }
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
    fmt.Printf("GLIB%d starting self triggers.\n", r.hw.Num)
    reply := make(chan data.ReqResp)
    p := ipbus.MakePacket(ipbus.Control)
    fmt.Printf("Starting random triggers.\n")
    timingcsrctrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    stat := r.hw.Module.Registers["csr"].Words["stat"]
    stat.Read(&p)
    timingcsrctrl.MaskedWrite("rand_int", 1, &p)
    timingcsrctrl.Read(&p)
    stat.Read(&p)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}

func (r *Reader) StopRandomTriggers() {
    reply := make(chan data.ReqResp)
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
    r.StopRandomTriggers()
}

func (r *Reader) Run(errs chan data.ErrPack) {
    fmt.Printf("Reader.Run() started.\n")
    defer r.exit.CleanExit("Reader.Run()")
    r.read = make(chan data.ReqResp, 100)
    running := true
    bufferlen := uint32(0)
    bufferdata, ok := r.hw.Module.Registers["buffer"].Ports["data"]
    if !ok {
        panic(fmt.Errorf("buffer.data port not found."))
    }
    buffersize, ok := r.hw.Module.Registers["buffer"].Words["count"]
    if !ok {
        panic(fmt.Errorf("buffer.count word not found."))
    }
    stat, ok := r.hw.Module.Registers["csr"].Words["stat"]
    if !ok {
        panic(fmt.Errorf("csr.stat word not found."))
    }
    timecontrol, ok := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("timing.csr.ctrl word not found."))
    }
    trigctr, ok := r.hw.Module.Modules["timing"].Registers["trig_ctr"]
    if !ok {
        panic(fmt.Errorf("timing.trig_ctr register not found."))
    }
    emptybufferdelay := 5 * time.Millisecond
    nempty := 0
    ndata := 0
    readout_stopped := uint32(7)
    for running {
        // Read up to X words of data then read size
        p := ipbus.MakePacket(ipbus.Control)
        if bufferlen > 0 {
            if bufferlen >= 255 {
                //fmt.Println("Read 255 words.")
                bufferdata.Read(255, &p)
                bufferlen -= 255
            }
            // Currently reducing the value a little from 108
            if bufferlen >= 100 {
                //fmt.Println("Read 100 mode words.")
                bufferdata.Read(100, &p)
            } else {
                //fmt.Printf("Read %d words.\n", bufferlen)
                bufferdata.Read(uint8(bufferlen), &p)
            }
        }
        buffersize.Read(&p)
        stat.Read(&p)
        timecontrol.Read(&p)
        trigctr.Read(1, &p)
        //fmt.Printf("Sending data/buffer len read packet.\n")
        r.hw.Send(p, r.read)
        select {
        // Signal to stop
        case stopped := <-r.Stop:
            running = false
            // Fudge for now, should rally wait until stuff is emptied before stopping
            stopped <- true
            break
        // Get replies from the read request, send data to writer's channel and
        // sleep for period based upon Number of words ready to read
        case data := <-r.read:
            if nempty == 300 {
                //fmt.Printf("300 empty reads, sending a software trigger.\n")
                //r.SendSoftwareTriggers(1)
            }
            if nempty > 0 && (nempty % 500 == 0) {
                stats := stat.GetReads(data)
                trigcnt := trigctr.GetReads(data)
                if len(stats) > 0 {
                    fmt.Printf("GLIB%d %d empty buffer packets. csr.stat = 0x%08x, %d triggers\n", r.hw.Num, nempty, stats[0], trigcnt[0])
                    ro_stop := stat.GetMaskedReads("ro_stop", data)[0]
                    if ro_stop > 0 {
                        fmt.Printf("ro_stop = 0x%x\n", ro_stop)
                        r.TrigStat()
                        for i := uint32(0); i < 76; i++ {
                            r.ChanStat(i)
                        }
                    }
                } else {
                    fmt.Printf("GLIB%d %d empty buffer packets but no stat read.\n", r.hw.Num, nempty)
                }
                stats = timecontrol.GetReads(data)
                if len(stats) > 0 {
                    fmt.Printf("GLIB%d %d empty buffer packets. timing.csr.ctrl = 0x%08x\n", r.hw.Num, nempty, stats[0])
                } else {
                    fmt.Printf("GLIB%d %d empty buffer packets but no timing.csr.ctrl read.\n", r.hw.Num, nempty)
                }
            }
            ro_stops := stat.GetMaskedReads("ro_stop", data)
            if len(ro_stops) > 0 {
                ro_stop := stat.GetMaskedReads("ro_stop", data)[0]
                if ro_stop != readout_stopped {
                    fmt.Printf("GLIB%d: ro_stop 0x%x -> 0x%x\n", r.hw.Num, readout_stopped, ro_stop)
                    /*
                    if ro_stop > 0 {
                        r.TrigStat()
                    }
                    */
                }
                readout_stopped = ro_stop
            } else {
                fmt.Printf("GLIB%d: Didn't get any read of csr.stat.ro_stop.\n", r.hw.Num)
            }
            lengths := buffersize.GetReads(data)
            n := len(lengths)
            if n > 0 {
                newlen := lengths[n - 1][0]
                if newlen == 0 && readout_stopped > 0 {
                    fmt.Printf("GLIB%d: buffer.count: %d -> %d\n", r.hw.Num, bufferlen, newlen)
                }
                bufferlen = newlen
            } else {
                fmt.Printf("Did not get read of buffer.count, keeping bufferlen = %d\n", bufferlen)
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

func (r Reader) Nuke() {
    csrctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    reply := make(chan data.ReqResp)
    fmt.Printf("Nuking GLIB%d.\n", r.hw.Num)
    p := ipbus.MakePacket(ipbus.Control)
    csrctrl.MaskedWrite("nuke", 1, &p)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
    time.Sleep(3 * time.Second)
}

func (r Reader) ResetBuffer() {
    timingcsrctrl, ok := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("timing.csr.ctrl word not found."))
    }
    fmt.Printf("Resetting timing buffers.\n")
    p := ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("buf_rst", 1, &p)
    timingcsrctrl.MaskedWrite("buf_rst", 0, &p)
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
    time.Sleep(time.Second)

}

func (r Reader) Reset(nuke bool) {
    if nuke {
        r.Nuke()
    }
    csrctrl, ok := r.hw.Module.Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("csr.ctrl word not found."))
    }
    csrstat, ok := r.hw.Module.Registers["csr"].Words["stat"]
    if !ok {
        panic(fmt.Errorf("csr.stat word not found."))
    }
    chanctrl, ok := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("chan_csr.ctrl word not found."))
    }
    idmagic, ok := r.hw.Module.Registers["id"].Words["magic"]
    if !ok {
        panic(fmt.Errorf("id.magic word not found."))
    }
    idinfo, ok := r.hw.Module.Registers["id"].Words["info"]
    if !ok {
        panic(fmt.Errorf("id.info word not found."))
    }
    timingcsrctrl, ok := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("timing.csr.ctrl word not found."))
    }
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
    rr = <-reply
    magics := idmagic.GetReads(rr)
    infos := idinfo.GetReads(rr)
    fmt.Printf("GLIB is alive: magic = 0x%x, info = 0x%x\n", magics[0][0], infos[0][0])
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
    // Reset timing
    fmt.Printf("Resetting timing.\n")
    p = ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("rst", 1, &p)
    timingcsrctrl.MaskedWrite("rst", 0, &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
    time.Sleep(time.Second)
    r.TrigStat()
    // Reset delays
    for _, ch := range r.channels {
        p = ipbus.MakePacket(ipbus.Control)
        csrctrl.MaskedWrite("chan_sel", ch, &p)
        chanctrl.MaskedWrite("sync_en", 1, &p)
        r.hw.Send(p, reply)
        rr = <-reply
        r.towrite <- rr
    }
    fmt.Printf("Resetting delay control.\n")
    p = ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("idelctrl_rst", 1, &p)
    timingcsrctrl.MaskedWrite("idelctrl_rst", 1, &p)
    timingcsrctrl.MaskedWrite("idelctrl_rst", 0, &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
    time.Sleep(time.Second)
    // Reset timing buffers (enable sync on all channels first)
    fmt.Printf("GLIB%d, setting csr.ctrl.board_id = %d\n", r.hw.Num, r.hw.Num)
    p = ipbus.MakePacket(ipbus.Control)
    csrctrl.MaskedWrite("board_id", uint32(r.hw.Num), &p)
    r.hw.Send(p, reply)
    rr = <-reply
    r.towrite <- rr
}

func (r Reader) PrepareSynchronisation() {
    csrctrl, ok := r.hw.Module.Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("csr.ctrl word not found."))
    }
    timingcsrctrl, ok := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("timing.csr.ctrl word not found."))
    }
    // Select external clock and external synchronisation
    fmt.Printf("Selecting external clock and enabling synchronisation.\n")
    p := ipbus.MakePacket(ipbus.Control)
    if r.hw.Num == 6 {
        fmt.Printf("GLIB6: using internal clock.\n")
        csrctrl.MaskedWrite("clk_sel", 0, &p)
        timingcsrctrl.MaskedWrite("sync_en", 0, &p)
        fmt.Printf("GLIB6 has no external clock to synchronise with.\n")
    } else {
        csrctrl.MaskedWrite("clk_sel", 1, &p)
        timingcsrctrl.MaskedWrite("sync_en", 1, &p)
    }
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
    r.towrite <- rr
}
func (r Reader) Clear() {
    fmt.Printf("Clearing buffer\n")
    timingcsrctrl, ok := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    if !ok {
        panic(fmt.Errorf("timing.csr.ctrl word not found."))
    }
    p := ipbus.MakePacket(ipbus.Control)
    timingcsrctrl.MaskedWrite("buf_rst", 1, &p)
    timingcsrctrl.MaskedWrite("buf_rst", 0, &p)
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
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

func (r Reader) TrigStat() {
    csrstat := r.hw.Module.Registers["csr"].Words["stat"]
    stat := r.hw.Module.Registers["trig"].Words["stat"]
    p := ipbus.MakePacket(ipbus.Control)
    stat.Read(&p)
    csrstat.Read(&p)
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
    clk_lock := csrstat.GetMaskedReads("clk_lock", rr)[0]
    clk_stop := csrstat.GetMaskedReads("clk_stop", rr)[0]
    ro_stop := csrstat.GetMaskedReads("ro_stop", rr)[0]
    buf_debug := csrstat.GetMaskedReads("buf_debug", rr)[0]
    fmt.Printf("csr.stat: clk_lock = %d, clk_stop = %d, ro_stop = 0x%x, buf_debug = 0x%x\n", clk_lock, clk_stop, ro_stop, buf_debug)
    bp0 := stat.GetMaskedReads("stat0.bp", rr)[0]
    err0 := stat.GetMaskedReads("stat0.err", rr)[0]
    derand_rdy0 := stat.GetMaskedReads("stat0.derand_rdy", rr)[0]
    derand_evt0 := stat.GetMaskedReads("stat0.derand_evt", rr)[0]
    derand_aempty0 := stat.GetMaskedReads("stat0.derand_aempty", rr)[0]
    derand_afull0 := stat.GetMaskedReads("stat0.derand_afull", rr)[0]
    derand_empty0 := stat.GetMaskedReads("stat0.derand_empty", rr)[0]
    derand_full0 := stat.GetMaskedReads("stat0.derand_full", rr)[0]
    fmt.Printf("GLIB%d, DEIMOS%d stat:\n", r.hw.Num, 0)
    fmt.Printf("    bp = %d, err = %d, derand_rdy = %d, derand_evt = %d\n", bp0, err0, derand_rdy0, derand_evt0)
    fmt.Printf("    derand_aempty = %d, derand_afull = %d, derand_empty = %d, derand_full = %d\n", derand_aempty0, derand_afull0, derand_empty0, derand_full0)
    bp1 := stat.GetMaskedReads("stat1.bp", rr)[0]
    err1 := stat.GetMaskedReads("stat1.err", rr)[0]
    derand_rdy1 := stat.GetMaskedReads("stat1.derand_rdy", rr)[0]
    derand_evt1 := stat.GetMaskedReads("stat1.derand_evt", rr)[0]
    derand_aempty1 := stat.GetMaskedReads("stat1.derand_aempty", rr)[0]
    derand_afull1 := stat.GetMaskedReads("stat1.derand_afull", rr)[0]
    derand_empty1 := stat.GetMaskedReads("stat1.derand_empty", rr)[0]
    derand_full1 := stat.GetMaskedReads("stat1.derand_full", rr)[0]
    fmt.Printf("GLIB%d, DEIMOS%d stat:\n", r.hw.Num, 1)
    fmt.Printf("    bp = %d, err = %d, derand_rdy = %d, derand_evt = %d\n", bp1, err1, derand_rdy1, derand_evt1)
    fmt.Printf("    derand_aempty = %d, derand_afull = %d, derand_empty = %d, derand_full = %d\n", derand_aempty1, derand_afull1, derand_empty1, derand_full1)
    r.towrite <- rr

}
func (r Reader) ChanStat(ch uint32) {
    csrctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    chanstat := r.hw.Module.Registers["chan_csr"].Words["stat"]
    p := ipbus.MakePacket(ipbus.Control)
    csrctrl.MaskedWrite("chan_sel", ch, &p)
    chanctrl.Read(&p)
    chanstat.Read(&p)
    reply := make(chan data.ReqResp)
    r.hw.Send(p, reply)
    rr := <-reply
    sync_en := chanctrl.GetMaskedReads("sync_en", rr)[0]
    phase := chanctrl.GetMaskedReads("phase", rr)[0]
    src_sel := chanctrl.GetMaskedReads("src_sel", rr)[0]
    invert := chanctrl.GetMaskedReads("invert", rr)[0]
    shift := chanctrl.GetMaskedReads("shift", rr)[0]
    ro_en := chanctrl.GetMaskedReads("ro_en", rr)[0]
    trig_en := chanctrl.GetMaskedReads("trig_en", rr)[0]
    stop_mask := chanctrl.GetMaskedReads("stop_mask", rr)[0]
    t_thresh := chanctrl.GetMaskedReads("t_thresh", rr)[0]
    bp := chanstat.GetMaskedReads("bp", rr)[0]
    err := chanstat.GetMaskedReads("err", rr)[0]
    derand_rdy := chanstat.GetMaskedReads("derand_rdy", rr)[0]
    derand_evt := chanstat.GetMaskedReads("derand_evt", rr)[0]
    derand_aempty := chanstat.GetMaskedReads("derand_aempty", rr)[0]
    derand_afull := chanstat.GetMaskedReads("derand_afull", rr)[0]
    derand_empty := chanstat.GetMaskedReads("derand_empty", rr)[0]
    derand_full := chanstat.GetMaskedReads("derand_full", rr)[0]
    idel_ctr := chanstat.GetMaskedReads("idel_ctr", rr)[0]
    fmt.Printf("GLIB%d channel %d stats:\n", r.hw.Num, ch)
    fmt.Printf("    sync_en = %d, phase = %d, src_sel = %d, invert = %d\n", sync_en, phase, src_sel, invert)
    fmt.Printf("    shift = %d, ro_en = %d, trig_en = %d, stop_mask = %d, t_thresh = %d\n", shift, ro_en, trig_en, stop_mask, t_thresh)
    fmt.Printf("    bp = %d, err = %d, derand_rdy = %d, derand_evt = %d\n", bp, err, derand_rdy, derand_evt)
    fmt.Printf("    derand_aempty = %d, derand_afull = %d, derand_empty = %d, derand_full = %d\n", derand_aempty, derand_afull, derand_empty, derand_full)
    fmt.Printf("    idel_ctr = %d\n", idel_ctr)
    r.towrite <- rr
}

func (r Reader) Align() {
    // Disable synchronous control on all channels, set delays on each 
    // channel then reenable syncrhonous control on all channels.
    csrctrl := r.hw.Module.Registers["csr"].Words["ctrl"]
    chanctrl := r.hw.Module.Registers["chan_csr"].Words["ctrl"]
    timectrl := r.hw.Module.Modules["timing"].Registers["csr"].Words["ctrl"]
    fmt.Printf("HW%d: aligning data channels.\n", r.hw.Num)
    for ich := uint32(0); ich < 76; ich++ {
        p := ipbus.MakePacket(ipbus.Control)
        csrctrl.MaskedWrite("chan_sel", ich, &p)
        chanctrl.MaskedWrite("sync_en", 0, &p)
        reply := make(chan data.ReqResp)
        r.hw.Send(p, reply)
        rr := <-reply
        r.towrite <- rr
    }
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
        r.ChanStat(ch.Channel)
    }
    for ich := uint32(0); ich < 76; ich++ {
        p := ipbus.MakePacket(ipbus.Control)
        csrctrl.MaskedWrite("chan_sel", ich, &p)
        chanctrl.MaskedWrite("sync_en", 1, &p)
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

func New(dir, store string, channels []uint32, exit *crash.Exit, inttrig, nuke bool) Control {
    c := Control{outpdir: dir, store: store, channels: channels, exit: exit, internaltrigger: inttrig, nuke: nuke}
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
    internaltrigger, nuke, started bool
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
        cfg := config.Load(hw.Num)
        r := NewReader(hw, cfg, c.datatowriter, time.Second, time.Microsecond, c.channels, c.exit)
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
func (c Control) Nuke() {
    fmt.Printf("Nuking GLIBs.\n")
    for i, reader := range c.readers {
        fmt.Printf("Nuking %dth GLIB.\n", i)
        reader.Nuke()
    }
}

func (c Control) Run(r data.Run) (bool, data.ErrPack) {
    /*
    Dave's instructions:
    Startup:

    - Make sure no triggers are being issued from the clock board
    - Use csr.ctrl.soft_rst to bring the registers to a known state
    - Check the clock is locked with csr.stat.clk_lock
    - Reset the sampling logic with timing.csr.ctrl.rst
    - Check that the channel derandomiser contents are all empty
    - Set up all the alignments, etc, and any other per-channel settings
    - Enable synchronous control on all channels of interest
    - Make sure the right window size, etc, is set up in csr.window_ctrl

    Then either for single-board mode:

    - Hit timing.csr.ctrl.buf_rst to align the buffers
    - Check the error status of all channels to make sure it's all locked

    or for multi-board mode:

    - Send a sync pulse from the clock board
    - Check the error status of all channels to make sure it's all locked

    Then:

    - Start the event readout loop
    - Start randoms if required
    - Program the trigger thresholds
    - Go through and enable trigger (not readout) for all channels of interest
    - Make sure readout is disabled for all channels not of interest (e.g. clock channels)
    - Enable readout for the channels of interest
    - Data should be flowing

    To shut down:

    - Disable readout on all channels.
    - Disable any source of random triggers
    - Read out the remaining buffer contents as per normal, until there's nothing left
    - Check that the channel derandomiser contents are all empty
    */
    // Make sure the clock board is not sending out triggers
    if !c.internaltrigger {
        fmt.Printf("Stopping any triggers from the clock board.")
        c.clock.Reset()
        c.clock.StopTriggers()
    }
    // Reset GLIBs
    fmt.Printf("Resetting data readers\n")
    for _, reader := range c.readers {
        reader.Reset(c.nuke)
        reader.TrigStat()
        reader.Align()
        reader.TriggerWindow(0xff, 0x2f)
        reader.SetCoincidenceMode(r.Coincidence)
    }
    // Synchronise GLIBs or reset buffer
    for _, reader := range c.readers {
        reader.PrepareSynchronisation()
    }
    if !c.internaltrigger {
        c.clock.SendTrigger()
    } else {
        for _, reader := range c.readers {
            reader.ResetBuffer()
        }
    }
    // Check everything is locked
    fmt.Printf("Checking that all GLIBs are correclty configured.\n")
    for _, reader := range c.readers {
        reader.TrigStat()
        for i := uint32(0); i < 76; i++ {
            reader.ChanStat(i)
        }
    }
    // Start readout 
    fmt.Printf("Starting readout routines.\n")
    for _, reader := range c.readers {
        go reader.Run(c.errs)
    }

    // Start random triggers
    fmt.Printf("Starting random triggers.\n")
    randrate := r.Rate
    if r.Threshold > 0 {
        randrate = 0.1
    }
    if !c.internaltrigger {
        c.clock.RandomRate(randrate)
        c.clock.StartTriggers()
    } else {
        for _, reader := range c.readers {
            reader.RandomTriggerRate(randrate)
            reader.StartRandomTriggers()
        }
    }
    // Start threshold triggers
    if r.Threshold > 0 {
        fmt.Printf("Starting threshold triggers.\n")
        for _, reader := range c.readers {
            reader.StartSelfTriggers(uint32(r.Threshold))
        }
    }
    // Enable readout
    fmt.Printf("Starting readout.\n")
    for _, reader := range c.readers {
        reader.EnableReadoutChannels()
    }
    // Start ticker
    fmt.Printf("Running for %v.\n", r.Duration)
    tick := time.NewTicker(r.Duration)
    quit := false
    err := data.MakeErrPack(error(nil))
    select {
    case <-tick.C:
        fmt.Printf("Stopped due to ticker.\n")
    case err = <-c.errs:
        fmt.Printf("Control.Run() stopped due to error channel\n")
        quit = true
    case <-c.signals:
        fmt.Printf("Control.Run() stopped by ctrl+c\n")
        quit = true
    }
    // Disable readout on all channels
    for _, reader := range c.readers {
        reader.DisableReadout()
    }
    // Stop random triggers
    if !c.internaltrigger {
        c.clock.StopTriggers()
    } else {
        for _, reader := range c.readers {
            reader.StopRandomTriggers()
        }
    }
    // Send signal to stop readout loop when empty
    stopped := make(chan bool, len(c.readers))
    for _, reader := range c.readers {
        reader.Stop <- stopped
    }
    // Wait for readers to have stopped
    nstopped := 0
    for nstopped < len(c.readers) {
        <-stopped
        nstopped += 1
    }

    // Check derandomisers are empty
    for _, reader := range c.readers {
        reader.TrigStat()

    }
    return quit, err




    /* BELOW OLD
    // Tell the writer to start a new file
    fmt.Printf("Starting run for %v.\n", r.Duration)
    c.runtowriter <- r
    time.Sleep(time.Second)
    // Tell the FPGAs to start acquisition
    c.startacquisition()
    for i, reader := range c.readers {
        fmt.Printf("Setting up %dth reader.\n", i)
        reader.Reset(c.nuke)
        reader.Align()
        reader.TriggerWindow(0xff, 0xf)
        reader.EnableReadoutChannels()
        time.Sleep(10 * time.Microsecond)
    }
    time.Sleep(time.Second)
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
    // If threshold is +ve then do a triggered run, otherwise do a random run
    quit := false
    err := data.MakeErrPack(error(nil))
    if r.Threshold >= 0 {
        randomrate := 0.1
        fmt.Printf("Running self triggers for %v.\n", r.Duration)
        tick := time.NewTicker(r.Duration)
        for _, reader := range c.readers {
            reader.Clear()
            go reader.Run(c.errs)
        }
        for _, reader := range c.readers {
            reader.SetCoincidenceMode(r.Coincidence)
            reader.StartSelfTriggers(uint32(r.Threshold))
        }
        if !c.internaltrigger {
            fmt.Printf("External triggers, starting from trigger board.\n")
            c.clock.RandomRate(randomrate)
            c.clock.StartTriggers()
        } else {
            for i, reader := range c.readers {
                fmt.Printf("Internal triggers: Start triggers for reader %d.\n", i)
                reader.RandomTriggerRate(randomrate)
                reader.StartRandomTriggers()
            }
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
        tick.Stop()
    } else { // run with random triggers
        if !c.internaltrigger {
            fmt.Printf("External triggers, starting from trigger board.\n")
            for _, reader := range c.readers {
                reader.Clear()
                go reader.Run(c.errs)
            }
            c.clock.RandomRate(r.Rate)
            c.clock.StartTriggers()
        } else {
            for i, reader := range c.readers {
                fmt.Printf("Internal triggers: Start triggers for reader %d.\n", i)
                reader.Clear()
                go reader.Run(c.errs)
                reader.RandomTriggerRate(r.Rate)
                reader.StopRandomTriggers()
                reader.StartRandomTriggers()
            }
        }
        fmt.Printf("Running random triggers for %v.\n", r.Duration)
        tick := time.NewTicker(r.Duration)
        select {
        case <-tick.C:
            fmt.Printf("Reducing random trigger rate due to ticker.\n")
        case err = <-c.errs:
            fmt.Printf("Control.Run() found an error.\n")
            quit = true
        case <-c.signals:
            fmt.Printf("Run stopped by ctrl-c.\n")
            quit = true
        }
        tick.Stop()
    }
    if !c.internaltrigger {
        c.clock.StopTriggers()
    }
    for _, reader := range c.readers {
        reader.StopTriggers()
        reader.DisableReadout()
    }
    // Stop the FPGAs
    // Really I should do this unless the error is something that would cause
    c.stopacquisition()
    // stop the readers
    for _, r := range c.readers {
        r.Stop <- true
    }
    return quit, err
    */
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
