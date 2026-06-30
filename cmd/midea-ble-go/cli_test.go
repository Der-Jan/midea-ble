package main

import (
	"reflect"
	"testing"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in   string
		cmd  string
		args []string
	}{
		{"temp 26", "temp", []string{"26"}},
		{"  MODE   cool  ", "mode", []string{"cool"}},
		{"on", "on", nil},
		{"", "", nil},
		{"   ", "", nil},
		{"swing ud", "swing", []string{"ud"}},
		{"Status", "status", nil},
	}
	for _, c := range cases {
		cmd, args := parseCommand(c.in)
		if cmd != c.cmd || !reflect.DeepEqual(args, c.args) {
			t.Errorf("parseCommand(%q)=(%q,%v) 期望 (%q,%v)", c.in, cmd, args, c.cmd, c.args)
		}
	}
}

func TestOnOff(t *testing.T) {
	if v, ok := onOff([]string{"on"}); !ok || !v {
		t.Error("on")
	}
	if v, ok := onOff([]string{"off"}); !ok || v {
		t.Error("off")
	}
	if _, ok := onOff([]string{"maybe"}); ok {
		t.Error("非法值应 ok=false")
	}
	if _, ok := onOff(nil); ok {
		t.Error("空参应 ok=false")
	}
}

// TestCommandTableWiring 确保命令表里的每条都接了实现，且无重名。
func TestCommandTableWiring(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commandList {
		if c.name == "" {
			t.Error("命令名为空")
		}
		if c.run == nil {
			t.Errorf("命令 %q 未接实现", c.name)
		}
		if seen[c.name] {
			t.Errorf("命令重名: %q", c.name)
		}
		seen[c.name] = true
	}
}
