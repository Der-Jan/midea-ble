package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/ac"
)

// knownDevice 是一条已验证可握手的设备记录（持久化在 ~/.midea-ble-go/known_devices.json）。
type knownDevice struct {
	SN           string `json:"sn"`
	UUID         string `json:"uuid"`
	AdvertisData string `json:"advertisData"`
	Handshake    bool   `json:"handshake"`
	VerifiedAt   string `json:"verifiedAt"`
}

// knownPath 返回已知设备注册表路径：~/.midea-ble-go/known_devices.json。
func knownPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "known_devices.json"
	}
	return filepath.Join(home, ".midea-ble-go", "known_devices.json")
}

func loadKnown() []knownDevice {
	b, err := os.ReadFile(knownPath())
	if err != nil {
		return nil
	}
	var list []knownDevice
	if json.Unmarshal(b, &list) != nil {
		return nil
	}
	return list
}

func saveKnown(list []knownDevice) error {
	p := knownPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// rememberDevice 把一台握手成功的设备 upsert 进注册表（按 UUID 去重）。
func rememberDevice(d ac.Device) {
	list := loadKnown()
	rec := knownDevice{
		SN:           d.SN,
		UUID:         d.UUID,
		AdvertisData: hexStr(d.AdvertisData),
		Handshake:    true,
		VerifiedAt:   time.Now().Format(time.RFC3339),
	}
	found := false
	for i := range list {
		if strings.EqualFold(list[i].UUID, d.UUID) {
			// 保留原有 adv / SN（若新值为空），更新验证信息
			if rec.AdvertisData == "" {
				rec.AdvertisData = list[i].AdvertisData
			}
			if rec.SN == "" {
				rec.SN = list[i].SN
			}
			list[i] = rec
			found = true
			break
		}
	}
	if !found {
		list = append(list, rec)
	}
	_ = saveKnown(list)
}

// resolveDevice 把用户输入的标识（UUID 或已知设备的 SN）解析为 (uuid, advHex)。
// 命中某条已知记录的 SN/UUID 时用其 UUID 与缓存的 advertisData；否则原样当 UUID。
func resolveDevice(arg string) (uuid, advHex string) {
	for _, d := range loadKnown() {
		if strings.EqualFold(d.SN, arg) || strings.EqualFold(d.UUID, arg) {
			return d.UUID, d.AdvertisData
		}
	}
	return arg, ""
}

// knownSet 返回已知设备 UUID（小写）集合，用于扫描时标记。
func knownSet() map[string]bool {
	m := map[string]bool{}
	for _, d := range loadKnown() {
		m[strings.ToLower(d.UUID)] = true
	}
	return m
}

// runKnown 实现 `known` 子命令：列出已保存的已知设备。
func runKnown(ctx context.Context, args []string) error {
	list := loadKnown()
	if len(list) == 0 {
		fmt.Printf("（暂无已知设备；握手成功的设备会自动记入 %s）\n", knownPath())
		return nil
	}
	fmt.Printf("已知设备（%s）：\n", knownPath())
	for _, d := range list {
		status := "握手:✓"
		if !d.Handshake {
			status = "握手:?"
		}
		fmt.Printf("  SN=%s  %s  %s  verified=%s  adv=%s\n", d.SN, d.UUID, status, d.VerifiedAt, d.AdvertisData)
	}
	return nil
}

func hexStr(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hexdigits[c>>4]
		out[i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}
