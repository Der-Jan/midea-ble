// midea-ble-go 从 macOS / Linux 经蓝牙直连控制美的 / 华凌空调，支持两种用法：
//   - 命令行参数式：midea-ble-go <子命令> [参数...]   （单次执行，便于脚本化）
//   - REPL 交互式：  midea-ble-go                      （无参时进入提示符）
//
// 子命令：scan / probe / known / status / on / off / set / handshake / shell / version。
// 不指定 --adv 时自动扫描重建 advertisData。
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]

	if len(args) == 0 {
		RunREPL(ctx) // 无参 → 进入 REPL
		return
	}
	if err := Dispatch(ctx, args); err != nil { // 带参 → 单次执行
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}
