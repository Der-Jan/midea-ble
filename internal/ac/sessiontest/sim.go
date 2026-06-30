// Package sessiontest 提供 AC 层测试夹具：一个用 proto 库充当"空调"的内存模拟器，
// 经 ac.Transport 与被测会话对话。ac 层的测试共用它，免重复。
// 它不被主程序导入，只在测试构建时编译进来（类似 net/http/httptest）。
package sessiontest

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/ac"
	"github.com/sorinyang/midea-ble-go/internal/proto"
)

// Adv 是构造的 advertisData（握手只要求 ≥11B）；OpenID 是固定的 6 字节 openID。
var (
	Adv    = mustHex("ac3132333435363738aabbccddeeff")
	OpenID = []byte{1, 2, 3, 4, 5, 6}
)

// Opts 返回加速的时序参数，便于测试快速跑完握手/重发。
func Opts() ac.Options {
	return ac.Options{
		C1Interval:   10 * time.Millisecond,
		StepInterval: 10 * time.Millisecond,
		BizInterval:  50 * time.Millisecond,
		BizReplyWait: 50 * time.Millisecond,
		Keepalive:    0,
		OpTimeout:    3 * time.Second,
		MaxReconnect: 1,
	}
}

// Dial 返回一个连接到全新内存模拟空调（使用 Adv）的 ac.Dialer。
func Dial() ac.Dialer {
	mt := &mockTransport{}
	newDeviceSim(Adv, mt)
	return func(ctx context.Context) (ac.Transport, error) { return mt, nil }
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// ---- mock transport（实现 ac.Transport）----

type mockTransport struct {
	mu      sync.Mutex
	notify  func([]byte)
	onWrite func([]byte)
	written [][]byte
}

var _ ac.Transport = (*mockTransport)(nil)

func (m *mockTransport) Write(p []byte) error {
	cp := append([]byte(nil), p...)
	m.mu.Lock()
	m.written = append(m.written, cp)
	cb := m.onWrite
	m.mu.Unlock()
	if cb != nil {
		cb(cp)
	}
	return nil
}
func (m *mockTransport) SetNotify(cb func([]byte)) { m.mu.Lock(); m.notify = cb; m.mu.Unlock() }
func (m *mockTransport) Close() error              { return nil }
func (m *mockTransport) feed(b []byte) {
	m.mu.Lock()
	cb := m.notify
	m.mu.Unlock()
	if cb != nil {
		cb(b)
	}
}

// ---- 设备侧模拟器（用 proto 库充当"空调"）----

func tmakeSum(b []byte) byte {
	s := 0
	for _, x := range b {
		s += int(x)
	}
	return byte((255 - s + 1) & 0xFF)
}

type deviceSim struct {
	rootKey    []byte
	devPri     []byte
	devPub     []byte
	sessionKey []byte
	seq        byte
	mt         *mockTransport

	run      int
	modeCode int
	tempInt  int
	half     bool
	wind     int
}

func newDeviceSim(adv []byte, mt *mockTransport) *deviceSim {
	rk, _ := proto.DeriveRootKey(adv)
	d := &deviceSim{rootKey: rk, mt: mt, run: 0, modeCode: 2, tempInt: 27, wind: 6}
	mt.onWrite = d.onWrite
	return d
}

func (d *deviceSim) nextSeq() byte {
	d.seq++
	if d.seq == 0 {
		d.seq = 1
	}
	return d.seq
}

func (d *deviceSim) respond(typ byte, key []byte, cmd byte, body []byte) {
	sec := proto.EncodeSecurity(cmd, body, d.nextSeq())
	enc, _ := proto.CipherMsg(key, sec)
	f, _ := proto.EncodeConn(typ, enc, d.nextSeq())
	d.mt.feed(f)
}

func (d *deviceSim) onWrite(p []byte) {
	typ, body, _, err := proto.DecodeConn(p)
	if err != nil {
		return
	}
	switch typ {
	case proto.T2:
		inner, err := proto.DecipherMsg(d.rootKey, body)
		if err != nil {
			return
		}
		cmd, sbody, _, _ := proto.DecodeSecurity(inner)
		switch cmd {
		case proto.C1:
			d.respond(proto.T2, d.rootKey, proto.C1, []byte{0}) // 要求重协商
		case proto.C2:
			if d.devPri == nil {
				pri, pub, _ := proto.CreateKeypair()
				d.devPri, d.devPub = pri, pub
			}
			d.respond(proto.T2, d.rootKey, proto.C2, d.devPub)
		case proto.C3:
			if len(sbody) >= 64 {
				sk, _ := proto.DeriveSessionKey(d.devPri, sbody[:64])
				d.sessionKey = sk
			}
			d.respond(proto.T2, d.rootKey, proto.C3, []byte{1})
		}
	case proto.T3:
		inner, err := proto.DecipherMsg(d.sessionKey, body)
		if err != nil {
			return
		}
		_, sbody, _, _ := proto.DecodeSecurity(inner)
		appliance, err := proto.DecodeBiz(sbody)
		if err != nil {
			return
		}
		d.applyControl(appliance)
		d.respond(proto.T3, d.sessionKey, proto.C4, proto.EncodeBiz(ac.BizTypeAC, d.buildStatus()))
	}
}

func (d *deviceSim) applyControl(a []byte) {
	if len(a) < 14 || a[0] != 0xAA || a[10] != 0x40 {
		return // 查询帧不改状态
	}
	d.run = int(a[11] & 1)
	d.modeCode = int((a[12] >> 5) & 7)
	d.tempInt = int(a[12]&0x0f) + 16
	d.half = a[12]&0x10 != 0
	d.wind = int(a[13] & 0x7f)
}

func (d *deviceSim) buildStatus() []byte {
	a := make([]byte, 33)
	a[0] = 0xAA
	a[2] = 0xAC
	a[8] = 8
	a[9] = 3
	S := a[10:]
	S[0] = 0xC0
	S[1] = byte(d.run & 1)
	S[2] = byte((d.modeCode << 5) | ((d.tempInt - 16) & 0x0f))
	if d.half {
		S[2] |= 0x10
	}
	S[3] = byte(d.wind)
	S[11] = byte(26*2 + 50) // 室内 26
	S[12] = byte(30*2 + 50) // 室外 30
	S[13] = byte((d.tempInt - 12) & 0x1f)
	a[1] = byte(len(a) - 1)
	a[len(a)-1] = tmakeSum(a[1 : len(a)-1])
	return a
}
