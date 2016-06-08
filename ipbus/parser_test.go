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

func TestParser(t *testing.T) {
	cm, err := NewCM("xml/testconnections.xml")
	if err != nil {
		t.Fatal(err)
		return
	}
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
