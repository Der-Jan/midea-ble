package main

// 一次性设备控制命令：handshake/status/on/off/set。
// 每条命令打开→握手→执行→关闭一台设备（一次性，不保活）。
// 参数形式：<cmd> <dev> [flags...]，其中 <dev> 为 UUID 或已知设备 SN。

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/sorinyang/midea-ble-go/internal/ac"
)

// newFlagSet 创建一个出错只返回 error（不自行打印用法、不 os.Exit）的 flagset，
// 让分发层统一输出「错误: ...」，避免 REPL 内重复刷屏或整进程退出。
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// openWithAdv 解析 <dev> 与可选 --adv，打开并握手一台设备（一次性，不保活）。
func openWithAdv(ctx context.Context, name string, args []string) (ac.IDevice, error) {
	if len(args) == 0 {
		return nil, errors.New("缺少设备参数 <dev>（UUID 或已知 SN）")
	}
	devArg := args[0]
	fs := newFlagSet(name)
	adv := fs.String("adv", "", "手动 advertisData(hex)")
	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}
	opt := ac.DefaultOptions()
	opt.Keepalive = 0 // 一次性命令不需要保活
	dev, _, err := openDevice(ctx, devArg, *adv, opt)
	return dev, err
}

func runHandshake(ctx context.Context, args []string) error {
	dev, err := openWithAdv(ctx, "handshake", args)
	if err != nil {
		return err
	}
	defer dev.Close()
	fmt.Println("✓ 握手成功")
	return nil
}

func runStatus(ctx context.Context, args []string) error {
	dev, err := openWithAdv(ctx, "status", args)
	if err != nil {
		return err
	}
	defer dev.Close()
	printStatus(dev.Snapshot(ctx))
	return nil
}

func runOn(ctx context.Context, args []string) error {
	dev, err := openWithAdv(ctx, "on", args)
	if err != nil {
		return err
	}
	defer dev.Close()
	report(dev.Power().Set(ctx, true))
	return nil
}

func runOff(ctx context.Context, args []string) error {
	dev, err := openWithAdv(ctx, "off", args)
	if err != nil {
		return err
	}
	defer dev.Close()
	report(dev.Power().Set(ctx, false))
	return nil
}

func runSet(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("缺少设备参数 <dev>（UUID 或已知 SN）")
	}
	devArg := args[0]
	fs := newFlagSet("set")
	adv := fs.String("adv", "", "手动 advertisData(hex)")
	mode := fs.String("mode", "", "模式 auto/cool/dry/heat/fan/smart_dry")
	temp := fs.Float64("temp", -1, "设定温度")
	fan := fs.String("fan", "", "风速 low/mid/high/full/mute/auto/fixed")
	swingUD := fs.Bool("swing-ud", false, "上下扫风")
	swingLR := fs.Bool("swing-lr", false, "左右扫风")
	eco := fs.Bool("eco", false, "ECO")
	strong := fs.Bool("strong", false, "强劲")
	noBeep := fs.Bool("no-beep", false, "静音")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	opt := ac.DefaultOptions()
	opt.Keepalive = 0
	dev, _, err := openDevice(ctx, devArg, *adv, opt)
	if err != nil {
		return err
	}
	defer dev.Close()

	dev.SetBeep(!*noBeep)
	report(dev.Power().Set(ctx, true)) // set 隐含开机
	if *mode != "" {
		report(dev.Mode().Set(ctx, *mode))
	}
	if *temp >= 0 {
		report(dev.Temperature().Set(ctx, *temp))
	}
	if *fan != "" {
		report(dev.Fan().Set(ctx, *fan))
	}
	if *swingUD || *swingLR {
		st, e := dev.Swing().Set(ctx, *swingUD, *swingLR)
		if e != nil {
			fmt.Printf("✗ %v\n", e)
		} else {
			fmt.Printf("✓ 上下:%v 左右:%v\n", st.UD, st.LR)
		}
	}
	if *eco {
		report(dev.Eco().Set(ctx, true))
	}
	if *strong {
		report(dev.Strong().Set(ctx, true))
	}
	printStatus(dev.Snapshot(ctx))
	return nil
}
