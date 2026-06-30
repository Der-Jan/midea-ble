// midea-ble-go CLI：命令表 + CLI/REPL 共用分发，外加已知设备注册表。
// 依赖 internal/ac（其中内置了会话引擎与 proto 协议栈），自身不直接接触 BLE。
//
// CLI 模式（带参数单次执行）与 REPL 交互模式（无参进入，有状态的多轮交互）
// 共用同一组业务函数；新增命令时在 commandList 加一行 & 实现 runXxx 即可。
package main

import (
	"context"
	"fmt"
)

// 由 ldflags 注入（见 Makefile）；未注入时为占位默认值。
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// command 是一条子命令，CLI 与 REPL 共用。
type command struct {
	name  string
	brief string
	run   func(ctx context.Context, args []string) error
}

// commandList 是全部子命令，顺序决定 help 与补全的展示顺序。
var commandList = []*command{
	{"scan", "扫描周围的美的/华凌空调（仅列出，不连接）", runScan},
	{"probe", "扫描后逐台握手探查，成功者入库（仅握手不控制）", runProbe},
	{"status", "查询并打印实时状态 status <dev> [--adv HEX]", runStatus},
	{"on", "开机 on <dev> [--adv HEX]", runOn},
	{"off", "关机 off <dev> [--adv HEX]", runOff},
	{"set", "设置 set <dev> [--mode] [--temp] [--fan] [--swing-ud] [--swing-lr] [--eco] [--strong] [--no-beep]", runSet},
	{"handshake", "仅验证握手 handshake <dev> [--adv HEX]", runHandshake},
	{"known", "列出已保存的已知设备", runKnown},
	{"version", "打印版本信息", runVersion},
}

func lookup(name string) (*command, bool) {
	for _, c := range commandList {
		if c.name == name {
			return c, true
		}
	}
	return nil, false
}

// Dispatch 解析一组参数并执行对应子命令。CLI 与 REPL 都走这里。
func Dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printHelp()
		return nil
	}
	c, ok := lookup(args[0])
	if !ok {
		return fmt.Errorf("未知命令 %q（输入 help 查看全部）", args[0])
	}
	return c.run(ctx, args[1:])
}

func printHelp() {
	fmt.Println("可用命令:")
	for _, c := range commandList {
		fmt.Printf("  %-12s %s\n", c.name, c.brief)
	}
	fmt.Println("  help         显示此帮助")
}

func runVersion(ctx context.Context, args []string) error {
	fmt.Printf("midea-ble-go %s (commit %s, built %s)\n", Version, Commit, BuildTime)
	return nil
}
