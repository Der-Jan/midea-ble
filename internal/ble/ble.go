package ble

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// BLE GATT 标识（子串匹配）：本设备的协议字节走 FFA1(写)/FFA2(通知)。
const (
	serviceUUIDPrefix = "ffa0"
	writeCharPrefix   = "ffa1"
	notifyCharPrefix  = "ffa2"
)

// Conn 是一条已连接 BLE 链路上的双向字节通道：write-with-response 写入 + 通知回调。
// 它结构化满足协议层（session）的 Transport 接口，但本包不反向依赖 session。
type Conn struct {
	dev    bluetooth.Device
	write  bluetooth.DeviceCharacteristic
	notify bluetooth.DeviceCharacteristic
	mu     sync.Mutex
	cb     func([]byte)
}

func (c *Conn) Write(p []byte) error {
	_, err := c.write.Write(p) // tinygo: CoreBluetooth 下为 write-with-response
	return err
}

func (c *Conn) SetNotify(cb func([]byte)) {
	c.mu.Lock()
	c.cb = cb
	c.mu.Unlock()
}

func (c *Conn) deliver(b []byte) {
	c.mu.Lock()
	cb := c.cb
	c.mu.Unlock()
	if cb != nil {
		cb(b)
	}
}

func (c *Conn) Close() error { return c.dev.Disconnect() }

// locate 扫描定位目标 UUID，返回其 Address 与最长 0x06A8 广播 payload（含 MAC），
// 并写入缓存供后续 Dial 复用。命中缓存（含 MAC 的完整包）则免再扫一次。
func locate(ctx context.Context, uuid string) (bluetooth.Address, []byte, error) {
	if err := enableAdapter(); err != nil {
		return bluetooth.Address{}, nil, err
	}
	if e, ok := lookupScan(uuid); ok && len(e.payload) >= 25 {
		return e.addr, e.payload, nil
	}
	var addr bluetooth.Address
	var best []byte
	found := false
	done := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(done) }) }

	go func() {
		select {
		case <-time.After(20 * time.Second):
		case <-ctx.Done():
		case <-done:
		}
		_ = adapter.StopScan()
	}()

	err := adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		if !strings.EqualFold(r.Address.String(), uuid) {
			return
		}
		addr = r.Address
		found = true
		if p := mfData06A8(r); len(p) > len(best) {
			best = append([]byte(nil), p...)
		}
		if len(best) >= 25 { // 拿到含 MAC 的完整包即可停
			stop()
		}
	})
	stop()
	if err != nil {
		return bluetooth.Address{}, nil, err
	}
	if !found {
		return bluetooth.Address{}, nil, fmt.Errorf("没扫到 %s", uuid)
	}
	cacheScan(uuid, addr, best) // 供后续 Dial 复用
	return addr, best, nil
}

// Locate 定位一台设备并重建其 advertisData，返回设备描述（地址已入缓存供 Dial 复用）。
// 广播 payload 不完整时 Device.AdvertisData 为 nil，由调用方决定是否要求手动指定。
func Locate(ctx context.Context, uuid string) (Device, error) {
	_, payload, err := locate(ctx, uuid)
	if err != nil {
		return Device{}, err
	}
	d := Device{UUID: uuid, AdvertisData: buildAdvertisData(payload)}
	if len(payload) >= 15 {
		d.SN = string(payload[1:15])
	}
	return d, nil
}

// Dial 连接到指定 UUID 的设备并发现 FFA1/FFA2，返回字节通道。
// 优先复用 Scan/Locate 缓存的地址，未命中则即时定位。
func Dial(ctx context.Context, uuid string) (*Conn, error) {
	addr, _, err := locate(ctx, uuid)
	if err != nil {
		return nil, err
	}
	dev, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	srvcs, err := dev.DiscoverServices(nil)
	if err != nil {
		_ = dev.Disconnect()
		return nil, err
	}
	c := &Conn{dev: dev}
	var gotW, gotN bool
	for _, sv := range srvcs {
		if !strings.Contains(strings.ToLower(sv.UUID().String()), serviceUUIDPrefix) {
			continue
		}
		chars, err := sv.DiscoverCharacteristics(nil)
		if err != nil {
			_ = dev.Disconnect()
			return nil, err
		}
		for _, ch := range chars {
			u := strings.ToLower(ch.UUID().String())
			if strings.Contains(u, writeCharPrefix) {
				c.write, gotW = ch, true
			}
			if strings.Contains(u, notifyCharPrefix) {
				c.notify, gotN = ch, true
			}
		}
	}
	if !gotW || !gotN {
		_ = dev.Disconnect()
		return nil, fmt.Errorf("未找到 FFA1/FFA2 特征")
	}
	if err := c.notify.EnableNotifications(c.deliver); err != nil {
		_ = dev.Disconnect()
		return nil, fmt.Errorf("订阅通知失败: %w", err)
	}
	return c, nil
}
