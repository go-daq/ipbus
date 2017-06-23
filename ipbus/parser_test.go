package ipbus

import (
	"testing"
)

func TestParserMissingFile(t *testing.T) {
	_, err := NewCM("missing.xml")
	if err == nil {
		t.Errorf("No error when trying to access a missing connection file.")
	}
}

func TestParserMissingTarget(t *testing.T) {
	cm, err := NewCM("xml/testconnections.xml")
	if err != nil {
		t.Error(err)
		return
	}
	_, err = cm.Target("MISSING")
	if err == nil {
		t.Errorf("No error when trying to find missing target from connection file.")
	}
}

func TestParser8chan(t *testing.T) {
	cm, err := NewCM("8chanxml/solidfpga.xml")
	if err != nil {
		t.Fatal(err)
	}
	target, err := cm.Target("SoLidFPGA")
	if err != nil {
		t.Fatal(err)
	}
	expectedregs := make(map[string]uint32)
	expectedregs["ctrl_reg"] = uint32(0x0)
	expectedregs["ctrl_reg.ctrl"] = uint32(0x0)
	expectedregs["ctrl_reg.id"] = uint32(0x2)
	expectedregs["ctrl_reg.stat"] = uint32(0x3)
	// chan at 0x8
	expectedregs["chan"] = uint32(0x8)
	expectedregs["chan.csr"] = uint32(0x8)
	expectedregs["chan.csr.ctrl"] = uint32(0x8)
	expectedregs["chan.csr.stat"] = uint32(0x9)
	expectedregs["chan.fifo"] = uint32(0xa)
	// io at 0x20
	expectedregs["io"] = uint32(0x20)
	expectedregs["io.csr"] = uint32(0x20)
	expectedregs["io.csr.ctrl"] = uint32(0x20)
	expectedregs["io.csr.stat"] = uint32(0x21)
	// io.freq_ctr at 0x24
	expectedregs["io.freq_ctr"] = uint32(0x24)
	expectedregs["io.freq_ctr.ctrl"] = uint32(0x24)
	expectedregs["io.freq_ctr.freq"] = uint32(0x25)
	// io.clock_i2c at 0x28
	expectedregs["io.clock_i2c"] = uint32(0x28)
	expectedregs["io.clock_i2c.ps_lo"] = uint32(0x28)
	expectedregs["io.clock_i2c.ps_hi"] = uint32(0x29)
	expectedregs["io.clock_i2c.ctrl"] = uint32(0x2a)
	expectedregs["io.clock_i2c.data"] = uint32(0x2b)
	expectedregs["io.clock_i2c.cmd_stat"] = uint32(0x2c)
	// io.spi a 0x30
	expectedregs["io.spi"] = uint32(0x30)
	expectedregs["io.spi.d0"] = uint32(0x30)
	expectedregs["io.spi.d1"] = uint32(0x31)
	expectedregs["io.spi.d2"] = uint32(0x32)
	expectedregs["io.spi.d3"] = uint32(0x33)
	expectedregs["io.spi.ctrl"] = uint32(0x34)
	expectedregs["io.spi.divider"] = uint32(0x35)
	expectedregs["io.spi.ss"] = uint32(0x36)
	// io.analog_i2c at 0x38
	expectedregs["io.analog_i2c"] = uint32(0x38)
	expectedregs["io.analog_i2c.ps_lo"] = uint32(0x38)
	expectedregs["io.analog_i2c.ps_hi"] = uint32(0x39)
	expectedregs["io.analog_i2c.ctrl"] = uint32(0x3a)
	expectedregs["io.analog_i2c.data"] = uint32(0x3b)
	expectedregs["io.analog_i2c.cmd_stat"] = uint32(0x3c)
	// timing at 0x40
	expectedregs["timing"] = uint32(0x40)
	expectedregs["timing.csr"] = uint32(0x40)
	expectedregs["timing.csr.ctrl"] = uint32(0x40)
	expectedregs["timing.sctr"] = uint32(0x42)
	expectedregs["timing.sctr.bottom"] = uint32(0x42)
	expectedregs["timing.sctr.top"] = uint32(0x43)
	foundregs := make(map[string]bool)
	for _, reg := range target.Regs {
		expected, ok := expectedregs[reg.Name]
		if !ok {
			t.Error("Unexpected register found: ", reg.Name)
		} else if reg.Addr != expected {
			t.Errorf("Register '%s' has address 0x%x, expected 0x%x", reg.Name, reg.Addr, expected)
		}
		foundregs[reg.Name] = true
	}
	for reg, _ := range expectedregs {
		if !foundregs[reg] {
			t.Errorf("Register '%s' not found.", reg)
		}
	}
}

func TestParser(t *testing.T) {
	cm, err := NewCM("xml/testconnections.xml")
	if err != nil {
		t.Fatal(err)
		return
	}
	t.Logf("Device list: %v\n", cm.Devices)
	target, err := cm.Target("GLIB")
	if err != nil {
		t.Fatal(err)
		return
	}
	expectedregs := make(map[string]bool)
	expectedregs["id"] = true
	expectedregs["id.magic"] = true
	expectedregs["id.info"] = true
	expectedregs["csr"] = true
	expectedregs["csr.ctrl"] = true
	expectedregs["csr.window_ctrl"] = true
	expectedregs["csr.stat"] = true
	expectedregs["chan"] = true
	expectedregs["timing"] = true
	expectedregs["timing.csr"] = true
	expectedregs["timing.csr.ctrl"] = true
	expectedregs["timing.csr.chan_ctrl"] = true
	expectedregs["timing.csr.stat"] = true
	expectedregs["timing.counter"] = true
	expectedregs["timing.counter.top"] = true
	expectedregs["timing.counter.bottom"] = true
	t.Log("Regs:\n")
	for _, reg := range target.Regs {
		t.Logf("\t%v\n", reg)
		if !expectedregs[reg.Name] {
			t.Error("Unexpected register found: ", reg.Name)
		}
	}
	for reg := range expectedregs {
		if _, ok := target.Regs[reg]; !ok {
			t.Error("Didn't find expected register: ", reg)
		}
	}
	if err != nil {
		t.Error(err)
	}
}
