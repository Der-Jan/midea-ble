package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/ac"
)

func runScan(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.Duration("timeout", 10*time.Second, "扫描时长")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Printf("[*] BLE 扫描 %s ...\n", *to)
	devs, err := ac.Scan(ctx, *to)
	if err != nil {
		return err
	}
	fmt.Printf("[*] 扫到 %d 台美的空调：\n", len(devs))
	for i, d := range devs {
		adv := ""
		if d.AdvertisData != nil {
			adv = fmt.Sprintf("%x", d.AdvertisData)
		}
		fmt.Printf("  [%d] SN=%s rssi=%d  %s  adv=%s\n", i, d.SN, d.RSSI, d.UUID, adv)
	}
	return nil
}

func runProbe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.Duration("timeout", 12*time.Second, "扫描时长")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opt := ac.DefaultOptions()
	opt.Keepalive = 0
	opt.OpTimeout = 25 * time.Second
	opt.MaxReconnect = 1
	fmt.Println("[*] 扫描并逐台握手探查（仅握手，不下发控制）…")
	results, err := ac.Probe(ctx, *to, opt)
	if err != nil {
		return err
	}
	ok := 0
	for _, r := range results {
		if r.OK {
			ok++
			rememberDevice(r.Device)
			fmt.Printf("  ✓ SN=%s rssi=%d %s\n", r.SN, r.RSSI, r.UUID)
		} else {
			fmt.Printf("  ✗ SN=%s rssi=%d %s  (%s)\n", r.SN, r.RSSI, r.UUID, r.Err)
		}
	}
	fmt.Printf("[*] 共 %d 台，握手成功 %d 台，成功者已入库 %s\n", len(results), ok, knownPath())
	return nil
}
