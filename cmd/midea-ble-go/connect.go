package main

import (
	"context"
	"fmt"

	"github.com/sorinyang/midea-ble-go/internal/ac"
)

// openDevice 解析设备标识（UUID 或已知 SN）、打开并握手，成功后记入已知注册表。
//   - arg：UUID 或已知设备 SN。
//   - advFlag：手动指定的 advertisData(hex)，为空时回退到已知缓存值。
//   - opt：会话参数（一次性命令通常 Keepalive=0；交互长连接用 ac.DefaultOptions()）。
//
// 返回已连接的设备；调用方负责 Close。握手失败时内部已 Close 并返回错误。
func openDevice(ctx context.Context, arg, advFlag string, opt ac.Options) (ac.IDevice, ac.Device, error) {
	uuid, knownAdv := resolveDevice(arg)
	advHex := advFlag
	if advHex == "" {
		advHex = knownAdv // 已知设备：复用缓存的 advertisData
	}
	dev, desc, err := ac.Open(ctx, uuid, advHex, nil, opt)
	if err != nil {
		return nil, ac.Device{}, err
	}
	if err := dev.Connect(ctx); err != nil {
		dev.Close()
		return nil, ac.Device{}, fmt.Errorf("握手失败: %w", err)
	}
	rememberDevice(desc) // 握手成功 → 记入已知设备
	return dev, desc, nil
}

// report 打印一个「返回值, error」型结果。
func report[T any](v T, err error) {
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		return
	}
	fmt.Printf("✓ %v\n", v)
}

// printStatus 打印一次状态快照。
func printStatus(st *ac.Status, err error) {
	if err != nil {
		fmt.Printf("✗ 读取失败: %v\n", err)
		return
	}
	pw := "关"
	if st.RunStatus == 1 {
		pw = "开"
	}
	fmt.Printf("    电源:%s 模式:%s 温度:%v℃ 风速:%s 室内:%v℃ 室外:%v℃ ECO:%d 强劲:%d 故障:%d\n",
		pw, st.ModeName, st.TempSet, windDisplay(st), st.TempIn, st.TempOut, st.EcoFunc, st.Strong, st.FaultFlag)
}

// windDisplay 在标准 1~7 风速时显示名称，否则显示设备上报的原始档位码（如强劲/空闲下的 100/102）。
func windDisplay(st *ac.Status) string {
	if st.WindSpeed >= 1 && st.WindSpeed <= 7 {
		return st.WindName
	}
	return fmt.Sprintf("档%d(设备自动)", st.WindSpeed)
}
