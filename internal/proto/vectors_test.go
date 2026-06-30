package proto

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

// 协议一致性向量：传输/安全层。

func TestRootKey(t *testing.T) {
	ad, _ := hex.DecodeString("ac3132333435363738aabbccddeeff")
	rk, err := DeriveRootKey(ad)
	if err != nil {
		t.Fatal(err)
	}
	hexEq(t, "rootKey", rk, "3ab82c346a77b6593d5ebe9f25d3cf50")
}

func TestFrameRoundtrip(t *testing.T) {
	raw, _ := EncodeConn(T1, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 7)
	typ, body, seq, err := DecodeConn(raw)
	if err != nil || typ != T1 || seq != 7 || len(body) != 10 {
		t.Fatalf("conn roundtrip: %v typ=%d seq=%d", err, typ, seq)
	}
	sec := EncodeSecurity(C2, []byte{1, 2, 3}, 3)
	cmd, b, sq, _ := DecodeSecurity(sec)
	if cmd != C2 || sq != 3 || len(b) != 3 {
		t.Fatal("security roundtrip")
	}
}

func TestAESCCMRoundtrip(t *testing.T) {
	key := make([]byte, 16)
	pt := []byte("hello midea bleapp 12345678")
	blob, err := CipherMsg(key, pt)
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) != len(pt)+16 {
		t.Fatalf("len=%d want %d", len(blob), len(pt)+16)
	}
	got, err := DecipherMsg(key, blob)
	if err != nil || string(got) != string(pt) {
		t.Fatalf("decipher: %v %q", err, got)
	}
}

func TestECDHAgrees(t *testing.T) {
	aPri, aPub, _ := CreateKeypair()
	bPri, bPub, _ := CreateKeypair()
	ska, err := DeriveSessionKey(aPri, bPub)
	if err != nil {
		t.Fatal(err)
	}
	skb, err := DeriveSessionKey(bPri, aPub)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(ska) != hex.EncodeToString(skb) {
		t.Fatalf("sessionKey mismatch %x %x", ska, skb)
	}
}
