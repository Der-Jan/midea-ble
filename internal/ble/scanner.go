// Package ble 是 BLE 层：调用本机蓝牙（经 tinygo.org/x/bluetooth：macOS→CoreBluetooth，
// Linux→BlueZ/D-Bus）执行扫描、定位、连接、收发字节。它无业务/协议语义，只认
// 美的 0x06A8 广播与 FFA1/FFA2 GATT 特征，向上把字节通道交给协议层（session）驱动。
package ble

import (
	"context"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// Device 是一次扫描得到的可连接空调描述。
type Device struct {
	UUID         string // macOS 设备标识（隐私 UUID，非真实 MAC）
	SN           string // 明文序列号（取自广播 payload）
	RSSI         int16
	AdvertisData []byte // 已重建的 HKDF ikm（0xAC+SN8+MAC逆序）；payload 不完整时为 nil
}

var adapter = bluetooth.DefaultAdapter

var (
	enableOnce sync.Once
	enableErr  error
)

// 进程内地址/广播缓存：扫描/定位期记录 UUID→(Address,payload)，使后续 Dial 免再扫一次。
type scanCacheEntry struct {
	addr    bluetooth.Address
	payload []byte
}

var (
	scanCacheMu sync.Mutex
	scanCache   = map[string]scanCacheEntry{}
)

func cacheScan(uuid string, addr bluetooth.Address, payload []byte) {
	scanCacheMu.Lock()
	scanCache[uuid] = scanCacheEntry{addr: addr, payload: append([]byte(nil), payload...)}
	scanCacheMu.Unlock()
}

func lookupScan(uuid string) (scanCacheEntry, bool) {
	scanCacheMu.Lock()
	defer scanCacheMu.Unlock()
	e, ok := scanCache[uuid]
	return e, ok
}

// enableAdapter 确保 adapter.Enable() 全进程只调用一次（tinygo 重复 Enable 会报
// "already calling Enable function"）。扫描/定位/连接各处共用。
func enableAdapter() error {
	enableOnce.Do(func() { enableErr = adapter.Enable() })
	return enableErr
}

// buildAdvertisData 从美的 0x06A8 厂商 payload 重建 HKDF 的 ikm：
// advertisData = 0xAC + SN8(payload[1:9]) + MAC(payload[19:25] 逆序)。
func buildAdvertisData(payload []byte) []byte {
	if len(payload) < 25 || payload[0] != 0x01 {
		return nil
	}
	sn8 := payload[1:9]
	mac := payload[19:25]
	out := make([]byte, 0, 15)
	out = append(out, 0xAC)
	out = append(out, sn8...)
	for i := len(mac) - 1; i >= 0; i-- {
		out = append(out, mac[i])
	}
	return out
}

func mfData06A8(r bluetooth.ScanResult) []byte {
	for _, e := range r.ManufacturerData() {
		if e.CompanyID == 0x06A8 {
			return e.Data
		}
	}
	return nil
}

// Scan 扫描周围美的(0x06A8)空调，按地址聚合最长广播包（含 MAC 的完整包）。
// 同时把地址+广播写入进程内缓存，供后续 Dial 免再扫。
func Scan(ctx context.Context, timeout time.Duration) ([]Device, error) {
	if err := enableAdapter(); err != nil {
		return nil, err
	}
	payloads := map[string][]byte{}
	rssi := map[string]int16{}
	sn := map[string]string{}

	go func() {
		select {
		case <-time.After(timeout):
		case <-ctx.Done():
		}
		_ = adapter.StopScan()
	}()

	err := adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		p := mfData06A8(r)
		if p == nil {
			return
		}
		addr := r.Address.String()
		if len(p) > len(payloads[addr]) {
			payloads[addr] = append([]byte(nil), p...)
			rssi[addr] = r.RSSI
			if len(p) >= 15 {
				sn[addr] = string(p[1:15])
			}
			cacheScan(addr, r.Address, p) // 缓存地址+广播，供后续 Dial 免再扫
		}
	})
	if err != nil {
		return nil, err
	}

	out := make([]Device, 0, len(payloads))
	for addr, p := range payloads {
		out = append(out, Device{
			UUID:         addr,
			SN:           sn[addr],
			RSSI:         rssi[addr],
			AdvertisData: buildAdvertisData(p),
		})
	}
	return out, nil
}
