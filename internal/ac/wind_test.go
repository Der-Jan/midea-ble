package ac

import "testing"

// 回归测试：风速协议枚举为 40低 60中 80高 100强劲 102自动。
// 早期实现曾误把 UI 按钮索引 1~7 当作线上值（含不存在的静音5/固定7），导致档位几乎不生效。

func TestWindEnumConstants(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"WindLow", WindLow, 40},
		{"WindMid", WindMid, 60},
		{"WindHigh", WindHigh, 80},
		{"WindFull", WindFull, 100},
		{"WindAuto", WindAuto, 102},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s=%d want %d", c.name, c.got, c.want)
		}
	}
}

// 映射表必须直接引用常量，防止 map 与常量定义再次割裂。
func TestWindMapsFromConstants(t *testing.T) {
	cases := []struct {
		code int
		name string
	}{
		{WindLow, "low"}, {WindMid, "mid"}, {WindHigh, "high"},
		{WindFull, "full"}, {WindAuto, "auto"},
	}
	for _, c := range cases {
		if got, ok := WindNames[c.code]; !ok || got != c.name {
			t.Errorf("WindNames[常量=%d]=%s/%v want %s", c.code, got, ok, c.name)
		}
	}
	for _, c := range cases {
		if got, ok := WindByName[c.name]; !ok || got != c.code {
			t.Errorf("WindByName[%q]=%d/%v want 常量 %d", c.name, got, ok, c.code)
		}
	}
}

func TestModeMapsFromConstants(t *testing.T) {
	cases := []struct {
		code int
		name string
	}{
		{ModeAuto, "auto"}, {ModeCool, "cool"}, {ModeDry, "dry"},
		{ModeHeat, "heat"}, {ModeFan, "fan"}, {ModeSmartDry, "smart_dry"},
	}
	for _, c := range cases {
		if got, ok := ModeNames[c.code]; !ok || got != c.name {
			t.Errorf("ModeNames[常量=%d]=%s/%v want %s", c.code, got, ok, c.name)
		}
	}
	for _, c := range cases {
		if got, ok := ModeByName[c.name]; !ok || got != c.code {
			t.Errorf("ModeByName[%q]=%d/%v want 常量 %d", c.name, got, ok, c.code)
		}
	}
}

// 臆造档位必须双向都查不到（Set 会据此拒绝下发）。
func TestWindRejectsFabricatedGears(t *testing.T) {
	for _, name := range []string{"mute", "fixed"} {
		if _, ok := WindByName[name]; ok {
			t.Errorf("WindByName[%q] 不应存在：该档位在协议中无依据", name)
		}
	}
	for _, code := range []int{5, 7} {
		if _, ok := WindNames[code]; ok {
			t.Errorf("WindNames[%d] 不应存在：5/7 是臆造的档位值", code)
		}
	}
}

func TestWindWireEncoding(t *testing.T) {
	// 名字 → 线上字节（BuildControlFrame a[13] & 127）。
	want := map[string]byte{
		"low": 40, "mid": 60, "high": 80, "full": 100, "auto": 102,
	}
	for name, wantByte := range want {
		code, ok := WindByName[name]
		if !ok {
			t.Fatalf("%s: map 中缺失", name)
		}
		s := NewACState()
		s.RunStatus = 1
		s.Mode = ModeCool
		s.TempSet = 26
		s.WindSpeed = code
		f, err := BuildControlFrame(s)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := f[13] & 127; got != wantByte {
			t.Errorf("%s: 线上字节=%d want %d", name, got, wantByte)
		}
	}
}
