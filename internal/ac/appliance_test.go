package ac

import (
	"encoding/hex"
	"testing"
)

func hexEq(t *testing.T, name string, got []byte, want string) {
	t.Helper()
	if g := hex.EncodeToString(got); g != want {
		t.Errorf("%s\n got=%s\nwant=%s", name, g, want)
	}
}

// AC 业务帧一致性向量。

func TestQueryFrame(t *testing.T) {
	f := BuildQueryFrame(1, 0)
	hexEq(t, "query", f, "aa17ac00000000000003412100ff03ff000200000001a92b")
}

func TestControlFrame26CoolOn(t *testing.T) {
	s := NewACState()
	s.RunStatus = 1
	s.Mode = ModeCool
	s.TempSet = 26
	s.WindSpeed = WindAuto
	s.BtnSound = 1
	s.Order = 1
	f, err := BuildControlFrame(s)
	if err != nil {
		t.Fatal(err)
	}
	hexEq(t, "control26", f,
		"aa24ac0000000000020240434a067f7fff300000009999999999000a0e8000000000019006")
}

func TestControlHalfDegree(t *testing.T) {
	s := NewACState()
	s.RunStatus = 1
	s.Mode = ModeCool
	s.TempSet = 26.5
	s.WindSpeed = WindAuto
	s.BtnSound = 0
	f, err := BuildControlFrame(s)
	if err != nil {
		t.Fatal(err)
	}
	if f[12] != 0x5A {
		t.Errorf("0.5℃ a[12]=%#x want 0x5a", f[12])
	}
}

func TestParseStatus(t *testing.T) {
	body, _ := hex.DecodeString("aa2aac00000000000803c0014b667f7f0000000000676c0f0037000000000000000000000000b7ff424c52")
	st, err := ParseStatusFrame(body)
	if err != nil {
		t.Fatal(err)
	}
	if st.RunStatus != 1 || st.ModeName != "cool" || st.TempSet != 27 {
		t.Fatalf("status: run=%d mode=%s temp=%v", st.RunStatus, st.ModeName, st.TempSet)
	}
	if st.TempIn != 26.7 || st.TempOut != 29.3 {
		t.Fatalf("temps: in=%v out=%v", st.TempIn, st.TempOut)
	}
}
