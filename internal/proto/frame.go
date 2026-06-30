// Package proto 实现 Midea BLE 协议的传输/安全层：三层帧编解码、密码学原语（HKDF、
// P-256 ECDH、AES-128-CCM）及握手状态机。设备无关，不包含任何设备类型的业务语义。
package proto

import (
	"errors"
	"fmt"
)

// conn 层 type
const (
	T1 = 0x01
	T2 = 0x02
	T3 = 0x03
)

// security 层 cmd
const (
	C1 = 0x01
	C2 = 0x02
	C3 = 0x03
	C4 = 0x04
)

func checksumNeg(b []byte) byte {
	var s int
	for _, x := range b {
		s += int(x)
	}
	return byte((-s) & 0xFF)
}

// ---- 连接层 conn: AA 55 LEN SEQ TYPE body CHK ----

// EncodeConn 编码连接层帧。
func EncodeConn(typ byte, body []byte, seq byte) ([]byte, error) {
	length := len(body) + 4
	if length > 255 {
		return nil, errors.New("conn body too long for 8-bit length")
	}
	out := make([]byte, 0, len(body)+6)
	out = append(out, 0xAA, 0x55, byte(length), seq, typ)
	out = append(out, body...)
	out = append(out, checksumNeg(out[2:])) // 覆盖 [2..end)
	return out, nil
}

// DecodeConn 解析连接层帧，返回 (type, body, seq)。
func DecodeConn(buf []byte) (typ byte, body []byte, seq byte, err error) {
	if len(buf) < 6 {
		return 0, nil, 0, fmt.Errorf("conn frame too short: %x", buf)
	}
	if buf[0] != 0xAA || buf[1] != 0x55 {
		return 0, nil, 0, fmt.Errorf("conn sync mismatch: %x", buf)
	}
	length := int(buf[2])
	seq = buf[3]
	typ = buf[4]
	bodyLen := length - 4
	if bodyLen < 0 || 5+bodyLen > len(buf) {
		return 0, nil, 0, fmt.Errorf("conn bad length: %x", buf)
	}
	body = append([]byte(nil), buf[5:5+bodyLen]...)
	return typ, body, seq, nil
}

// ---- 安全层 security: CMD SEQ LEN body（无 checksum）----

// EncodeSecurity 编码安全层帧。
func EncodeSecurity(cmd byte, body []byte, seq byte) []byte {
	out := make([]byte, 0, len(body)+3)
	out = append(out, cmd, seq, byte(len(body)))
	out = append(out, body...)
	return out
}

// DecodeSecurity 解析安全层帧，返回 (cmd, body, seq)。
func DecodeSecurity(buf []byte) (cmd byte, body []byte, seq byte, err error) {
	if len(buf) < 3 {
		return 0, nil, 0, fmt.Errorf("security frame too short: %x", buf)
	}
	cmd, seq = buf[0], buf[1]
	length := int(buf[2])
	end := 3 + length
	if end > len(buf) {
		end = len(buf)
	}
	body = append([]byte(nil), buf[3:end]...)
	return cmd, body, seq, nil
}

// ---- 业务层 biz: TYPE LEN 00 body CHK ----

// EncodeBiz 编码业务层帧（type|len|reserved|body|chk，校验覆盖 [0..len-2)）。
func EncodeBiz(typ byte, body []byte) []byte {
	length := len(body) + 4
	out := make([]byte, length)
	out[0] = typ
	out[1] = byte(length)
	// out[2] 预留为 0
	copy(out[3:], body)
	out[length-1] = checksumNeg(out[:length-1])
	return out
}

// DecodeBiz 取出 body（JS: slice(3, len-1)）。
func DecodeBiz(buf []byte) ([]byte, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("biz frame too short: %x", buf)
	}
	return append([]byte(nil), buf[3:len(buf)-1]...), nil
}
