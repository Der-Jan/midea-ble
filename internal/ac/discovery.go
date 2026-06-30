// Package ac 是业务层：在会话引擎（Session）之上，按功能模块（电源/模式/温度/风速/
// 扫风/ECO/强劲/传感器）暴露读写双向能力，并作为门面把 BLE 层（ble）的扫描/连接
// 装配成一台 IDevice。调用方（cli）只依赖本包。
package ac

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/ble"
)

// Device 是一次扫描得到的设备描述（来自 BLE 层，业务层透传给调用方）。
type Device = ble.Device

// 编译期确认 BLE 层的字节通道满足协议层的 Transport（结构化）。
var _ Transport = (*ble.Conn)(nil)

// Scan 扫描周围的美的/华凌空调（仅发现，不连接）。
func Scan(ctx context.Context, timeout time.Duration) ([]Device, error) {
	return ble.Scan(ctx, timeout)
}

// Open 定位并构造一台空调设备（尚未连接，调用方再 Connect）。
// uuid：macOS 设备标识；advHex 非空则用它做 advertisData，否则从广播重建；
// openID6 可传 nil（随机 6 字节）。同时返回该设备的描述，便于记录为已知设备。
func Open(ctx context.Context, uuid, advHex string, openID6 []byte, opt Options) (IDevice, Device, error) {
	found, err := ble.Locate(ctx, uuid)
	if err != nil {
		return nil, Device{}, err
	}
	adv := found.AdvertisData
	if advHex != "" {
		if adv, err = hex.DecodeString(advHex); err != nil {
			return nil, Device{}, fmt.Errorf("advHex 非法: %w", err)
		}
	} else if adv == nil {
		return nil, Device{}, fmt.Errorf("广播 payload 不完整，未含 MAC；多试几次或用 --adv")
	}
	if len(openID6) != 6 {
		openID6 = randID6()
	}
	desc := Device{UUID: uuid, SN: found.SN, AdvertisData: adv}
	dial := func(ctx context.Context) (Transport, error) { return ble.Dial(ctx, uuid) }
	sess, err := NewSession(dial, adv, openID6, opt)
	if err != nil {
		return nil, desc, err
	}
	return NewDevice(sess), desc, nil
}

// ProbeResult 是对一台设备的握手探查结果。
type ProbeResult struct {
	Device
	OK  bool
	Err string
}

// Probe 扫描一次，随后对每台可重建 advertisData 的设备依次尝试握手（复用扫描期
// 捕获的地址，不逐台重扫），返回每台的握手结果。仅握手后立即断开，不下发控制。
func Probe(ctx context.Context, scanTimeout time.Duration, opt Options) ([]ProbeResult, error) {
	devs, err := ble.Scan(ctx, scanTimeout)
	if err != nil {
		return nil, err
	}
	out := make([]ProbeResult, 0, len(devs))
	for _, d := range devs {
		if d.AdvertisData == nil {
			out = append(out, ProbeResult{Device: d, OK: false, Err: "广播格式无法重建 advertisData"})
			continue
		}
		ok, herr := tryHandshake(ctx, d.UUID, d.AdvertisData, opt)
		pr := ProbeResult{Device: d, OK: ok}
		if herr != nil {
			pr.Err = herr.Error()
		}
		out = append(out, pr)
	}
	return out, nil
}

func tryHandshake(ctx context.Context, uuid string, adv []byte, opt Options) (bool, error) {
	dial := func(ctx context.Context) (Transport, error) { return ble.Dial(ctx, uuid) }
	sess, err := NewSession(dial, adv, randID6(), opt)
	if err != nil {
		return false, err
	}
	defer sess.Close()
	if err := sess.Handshake(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func randID6() []byte {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return b
}
