package proto

import (
	"crypto/rand"
	"fmt"
)

// HandshakeState 握手状态机（出包构造 + 入包解析）。
type HandshakeState struct {
	AdvertisData []byte
	OpenID6      []byte
	RootKey      []byte
	SessionKey   []byte
	MyPri        []byte
	MyPub64      []byte
	PeerPub64    []byte

	connSeq byte
	secSeq  byte
}

func randByte1to255() byte {
	b := make([]byte, 1)
	rand.Read(b)
	if b[0] == 0 {
		return 1
	}
	return b[0]
}

// NewHandshakeState 用广播数据与 openId6 初始化（自动派生 rootKey）。
func NewHandshakeState(advertisData, openID6 []byte) (*HandshakeState, error) {
	if len(advertisData) < 11 {
		return nil, fmt.Errorf("advertisData 太短: %d", len(advertisData))
	}
	if len(openID6) != 6 {
		return nil, fmt.Errorf("openID6 必须 6 字节")
	}
	rk, err := DeriveRootKey(advertisData)
	if err != nil {
		return nil, err
	}
	return &HandshakeState{
		AdvertisData: advertisData,
		OpenID6:      openID6,
		RootKey:      rk,
		connSeq:      randByte1to255(),
		secSeq:       randByte1to255(),
	}, nil
}

func (h *HandshakeState) nextConnSeq() byte {
	h.connSeq++
	if h.connSeq == 0 {
		h.connSeq = 1
	}
	return h.connSeq
}

func (h *HandshakeState) nextSecSeq() byte {
	h.secSeq++
	if h.secSeq == 0 {
		h.secSeq = 1
	}
	return h.secSeq
}

// BuildGetVersion conn t1 + 固定 body。
func (h *HandshakeState) BuildGetVersion() ([]byte, error) {
	return EncodeConn(T1, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0}, h.nextConnSeq())
}

// BuildC1: body=openId6，rootKey 加密，外层 t2。
func (h *HandshakeState) BuildC1() ([]byte, error) {
	sec := EncodeSecurity(C1, h.OpenID6, h.nextSecSeq())
	enc, err := CipherMsg(h.RootKey, sec)
	if err != nil {
		return nil, err
	}
	return EncodeConn(T2, enc, h.nextConnSeq())
}

// BuildC2: 空 body，rootKey 加密。
func (h *HandshakeState) BuildC2() ([]byte, error) {
	sec := EncodeSecurity(C2, nil, h.nextSecSeq())
	enc, err := CipherMsg(h.RootKey, sec)
	if err != nil {
		return nil, err
	}
	return EncodeConn(T2, enc, h.nextConnSeq())
}

// BuildC3: body=pubKey(64)||cipherMsg(sessionKey, advertisData)，rootKey 加密。
func (h *HandshakeState) BuildC3() ([]byte, error) {
	if h.SessionKey == nil {
		return nil, fmt.Errorf("需先完成 c2 才能发 c3")
	}
	inner, err := CipherMsg(h.SessionKey, h.AdvertisData)
	if err != nil {
		return nil, err
	}
	body := append(append([]byte(nil), h.MyPub64...), inner...)
	sec := EncodeSecurity(C3, body, h.nextSecSeq())
	enc, err := CipherMsg(h.RootKey, sec)
	if err != nil {
		return nil, err
	}
	return EncodeConn(T2, enc, h.nextConnSeq())
}

// BuildBiz: c4 业务命令，sessionKey 加密，外层 t3。
func (h *HandshakeState) BuildBiz(bizType byte, bizBody []byte) ([]byte, error) {
	if h.SessionKey == nil {
		return nil, fmt.Errorf("需先完成握手")
	}
	biz := EncodeBiz(bizType, bizBody)
	sec := EncodeSecurity(C4, biz, h.nextSecSeq())
	enc, err := CipherMsg(h.SessionKey, sec)
	if err != nil {
		return nil, err
	}
	return EncodeConn(T3, enc, h.nextConnSeq())
}

// RecvInfo 入包解析结果。
type RecvInfo struct {
	Kind      string // t1/c1/c2/c3/biz/sec_error/unknown
	Result    int    // c1/c3 的 result（-1 表示无）
	PeerPub64 []byte // c2
	BizBody   []byte // biz
	Body      []byte
}

// OnRecv 解析一个完整 conn 帧。
func (h *HandshakeState) OnRecv(raw []byte) (*RecvInfo, error) {
	typ, body, _, err := DecodeConn(raw)
	if err != nil {
		return nil, err
	}
	info := &RecvInfo{Result: -1}
	if typ == T1 {
		info.Kind = "t1"
		info.Body = body
		return info, nil
	}
	if (typ == T2 || typ == T3) && len(body) < 16 {
		info.Kind = "sec_error" // 如 ff04/ff05
		info.Body = body
		return info, nil
	}
	var inner []byte
	switch typ {
	case T2:
		inner, err = DecipherMsg(h.RootKey, body)
	case T3:
		inner, err = DecipherMsg(h.SessionKey, body)
	default:
		info.Kind = "unknown"
		info.Body = body
		return info, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decrypt fail: %w", err)
	}
	cmd, sbody, _, err := DecodeSecurity(inner)
	if err != nil {
		return nil, err
	}
	switch cmd {
	case C1:
		info.Kind = "c1"
		if len(sbody) >= 1 {
			info.Result = int(sbody[0])
		}
	case C2:
		info.Kind = "c2"
		info.PeerPub64 = sbody
	case C3:
		info.Kind = "c3"
		if len(sbody) >= 1 {
			info.Result = int(sbody[0])
		}
	case C4:
		info.Kind = "biz"
		info.BizBody, _ = DecodeBiz(sbody)
	default:
		info.Kind = "unknown_sec"
		info.Body = sbody
	}
	return info, nil
}
