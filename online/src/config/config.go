package config

import (
    "errors"
    "fmt"
    "io/ioutil"
    "encoding/json"
//    "os"
)

func NewGLIB(alignmentfn, pedspafn, maskfn string) {
    data, err := ioutil.ReadFile(alignmentfn)
    if err != nil {
        panic(err)
    }
    chandelays := []TimingOffset{}
    err = json.Unmarshal(data, &chandelays)
    if err != nil {
        panic(err)
    }
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
    Phase, Invert uint32
    Shift, Increment uint32
    Pedestal, SPA float64
    Threshold uint32
}

func (dc *DataChannel) merge(offset TimingOffset, pedspa PedSPA, masks Masks) error {
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
    for _, ch := range masks.ReadoutDisabled {
        if ch.GLIB == dc.GLIB && ch.Channel == dc.Channel {
            dc.ReadoutEnabled = false
        }
    }
    for _, ch := range masks.TriggerDisabled {
        if ch.GLIB == dc.GLIB && ch.Channel == dc.Channel {
            dc.TriggerEnabled = false
        }
    }
    return error(nil)
}

func (dc *DataChannel) SetThreshold(t uint32) {
    dc.Threshold = t
}

func (dc *DataChannel) CalcThreshold(tspa float64) {
    thr := dc.Pedestal + tspa * dc.SPA
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
