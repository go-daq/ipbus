package main

import (
    "fmt"
    "io/ioutil"
    "encoding/json"
//    "os"
)

func NewGLIB(alignmentfn, pedspafn, maskfn) {

}

type Glib struct {
    Module int
    DataChannels []*DataChannel
    TriggerChannels []uint32
}

func (g Glib) SetThresholds(thr uint32) {
    for _, ch := range g.DataChannels {
        ch.SetThreshold(thr)
    }
}

func (g Glib) CalcThresholds(thr float64) {
    for _, ch := range g.DataChannels {
        ch.CalcThreshold(thr)
    }
}

type DataChannel struct {
    ChanID
    TriggerEnabled, ReadoutEnabled bool
    Phase, Invert bool
    Shift, Offset int
    Pedestal, SPA float64
    Threshold uint32
}

func (dc *DataChannel) merge(offset ChannelTimingOffset, pedspa ChannelPedSPA, masks CannelMasks) error {
    if offset.GLIB != dc.GLIB || offset.Channel != dc.Channel {
        return errors.New("Channel merge fail: offset from different channel")
    }
    dc.Phase = offset.Phase
    dc.Invert = offset.Invert
    dc.Shift = offset.Shift
    dc.Increment = offset.Increment
    if pedspa.GLIB != dc.GLIB || pedspa.Channel != dc.Channel {
        return errors.New("Channel merge fail: ped and SPA from different channel")
    }
    dc.Pedestal = pedspa.Pedestal
    dc.SPA = pedspa.SinglePixelAmplitude
    dc.ReadoutEnabled = true
    dc.TriggerEnabled = true
    for _, ch := range masks.ReadoutDisable {
        if ch.GLIB == dc.GLIB && ch.Channel = dc.Channel {
            dc.ReadoutEnabled = false
        }
    }
    for _, ch := range masks.TriggerDisable {
        if ch.GLIB == dc.GLIB && ch.Channel = dc.Channel {
            dc.TriggerEnabled = false
        }
    }
}

func (dc *DataChannel) SetThreshold(t uint32) {
    dc.Threshold = t
}

func (dc *DataChannel) CalcThreshold(tspa float64) {
    thr := dc.Pdestal + tspa * dc.SPA
    dc.Threshold = uint32(thr)
}

type ChanID struct {
    GLIB, Channel uint32
}

func (c ChanID) String() string {
    return fmt.Sprintf("GLIB%d chan %d", c.GLIB, c.Channel)
}

type Masks struct {
    ReadoutDisabled, TriggerDisabled []ChanID
}

type PedSPA struct {
    ChanID
    Pedestal, SinglePixelAmplitude float64
}

type TimingOffset struct {
    ChanID
    Phase, Invert uint32
    Shift, Increment uint32
}

func (o TimingOffset) String() string {
    return fmt.Sprintf("%s: inv = %d, phase = %d, shift = %d, incs = %d",
                        o.ChanID.String(), o.Invert, o.Phase, o.Shift,
                        o.Increment)
}

func main() {
    data, err := ioutil.ReadFile("align_GLIB6.json")
    if err != nil {
        panic(err)
    }
    chandelays := []ChannelTimingOffset{}
    err = json.Unmarshal(data, &chandelays)
    if err != nil {
        panic(err)
    }
    fmt.Println("Read channels:")
    for _, ch := range chandelays {
        fmt.Printf("    %v\n", ch)
    }
}
