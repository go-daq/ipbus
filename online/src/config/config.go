package config

import (
    "errors"
    "fmt"
    "io/ioutil"
    "encoding/json"
//    "os"
)

func Load(module int) Glib {
    alignmentfn := fmt.Sprintf("align_GLIB%d.json", module)
    pedspafn := fmt.Sprintf("pedspa_GLIB%d.json", module)
    maskfn := fmt.Sprintf("masks_GLIB%d.json", module)
    return NewGLIB(module, alignmentfn, pedspafn, maskfn)

}

func NewGLIB(module int, alignmentfn, pedspafn, maskfn string) Glib {
    data, err := ioutil.ReadFile(alignmentfn)
    if err != nil {
        panic(err)
    }
    chandelays := []TimingOffset{}
    err = json.Unmarshal(data, &chandelays)
    if err != nil {
        panic(err)
    }
    data, err = ioutil.ReadFile(pedspafn)
    if err != nil {
        panic(err)
    }
    chanpedspas := []PedSPA{}
    err = json.Unmarshal(data, &chanpedspas)
    if err != nil {
        panic(err)
    }
    data, err = ioutil.ReadFile(maskfn)
    if err != nil {
        panic(err)
    }
    masks := Masks{}
    err = json.Unmarshal(data, &masks)
    if err != nil {
        panic(err)
    }
    chans := []*DataChannel{}
    for i := uint32(0); i < 76; i++ {
        id := ChanID{GLIB: uint32(module), Channel: i}
        dc := &DataChannel{ChanID: id}
        foundoffset := false
        offset := TimingOffset{}
        for _, off := range chandelays {
            if off.GLIB == dc.GLIB && off.Channel == dc.Channel {
                offset = off
                foundoffset = true
            }
        }
        if !foundoffset {
            continue
        }
        pedspa := PedSPA{}
        for _, p := range chanpedspas {
            if p.GLIB == dc.GLIB && p.Channel == dc.Channel {
                pedspa = p
            }
        }
        err := dc.merge(offset, pedspa, masks)
        if err != nil {
            panic(err)
        }
        chans = append(chans, dc)
    }
    triggers := []uint32{0x90, 0x91}
    g := Glib{Module: module, DataChannels: chans, TriggerChannels: triggers}
    return g
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

func (g Glib) GetThreshold(channel uint32) (uint32, error) {
    for _, ch := range g.DataChannels {
        if ch.Channel == channel {
            return ch.Threshold, error(nil)
        }
    } 
    return uint32(0x3fff), fmt.Errorf("Unknown channel: %d", channel)
}

type DataChannel struct {
    ChanID
    TriggerEnabled, ReadoutEnabled bool
    Phase, Invert uint32
    Shift, Increment uint32
    Pedestal, SPA float64
    Threshold uint32
}

func (d DataChannel) String() string {
    msg := fmt.Sprintf("%s: trigger = %t, readout = %t. ", d.ChanID.String(), d.TriggerEnabled, d.ReadoutEnabled)
    msg += fmt.Sprintf("ph%d, inv%d, sh%d, inc%d. ", d.Phase, d.Invert, d.Shift, d.Increment)
    msg += fmt.Sprintf("Ped = %f, SPA = %f.", d.Pedestal, d.SPA)
    msg += fmt.Sprintf("Threshold = %d.", d.Threshold)
    return msg
}

func (dc *DataChannel) merge(offset TimingOffset, pedspa PedSPA, masks Masks) error {
    fmt.Printf("Merging %v, %v and %v\n", offset, pedspa, masks)
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
