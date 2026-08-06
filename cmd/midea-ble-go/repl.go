package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	prompt "github.com/c-bata/go-prompt"

	"github.com/sorinyang/midea-ble-go/internal/ac"
)

// replSession holds the REPL's mutable state that persists across commands:
// idle mode (dev==nil) → scan/connect → connected mode → disconnect → idle.
type replSession struct {
	dev      ac.IDevice  // nil when not connected
	sn       string      // device display name (SN or short UUID)
	lastScan []ac.Device // most recent scan results, for "connect <idx>"
}

func (s *replSession) reset() {
	if s.dev != nil {
		s.dev.Close()
	}
	s.dev = nil
	s.sn = ""
}

func (s *replSession) prompt() string {
	if s.dev != nil {
		return s.sn + "> "
	}
	return "midea-ble-go> "
}

// ---- entry ----

// RunREPL starts the top-level interactive prompt (entered when midea-ble-go
// is launched with no arguments). It maintains a session across commands:
// scan → connect → operate → disconnect → scan again.
//
// stdin not a terminal (pipe/redirect) → fall back to stateless line-by-line
// Dispatch, mirroring CLI behaviour.
func RunREPL(ctx context.Context) {
	if !stdinIsTerminal() {
		runPiped(ctx)
		return
	}
	s := &replSession{}
	defer s.reset()
	fmt.Println("midea-ble-go 交互模式（Tab 补全 / ↑↓ 历史）")
	fmt.Println("  scan → connect <编号> → 操作 → disconnect → scan ...")
	fmt.Println("  help 查看命令，exit 退出")
	fmt.Println()
	p := prompt.New(
		func(line string) { s.execute(ctx, line) },
		func(d prompt.Document) []prompt.Suggest { return s.complete(d) },
		prompt.OptionPrefix("midea-ble-go> "),
		prompt.OptionTitle("midea-ble-go"),
		prompt.OptionLivePrefix(func() (string, bool) { return s.prompt(), true }),
	)
	p.Run()
}

// ---- dispatcher ----

func (s *replSession) execute(ctx context.Context, line string) {
	cmd, args := parseCommand(line)
	if s.dev != nil {
		s.execConnected(ctx, cmd, args)
	} else {
		s.execIdle(ctx, cmd, args)
	}
}

// ---- idle mode ----

const idleHelp = `命令（idle）：
  scan [--timeout DUR]       扫描周围空调（记录结果供 connect 用）
  connect <编号|SN|UUID>     连接到一台设备
  probe [--timeout DUR]      扫描后逐台握手探查，成功者入库
  known                      列出已知设备
  version                    打印版本信息
  help                       显示此帮助
  exit | quit                退出程序`

func (s *replSession) execIdle(ctx context.Context, cmd string, args []string) {
	switch cmd {
	case "", "help", "h", "?":
		fmt.Println(idleHelp)
	case "exit", "quit":
		s.reset()
		os.Exit(0)
	case "scan":
		s.cmdScan(ctx, args)
	case "probe":
		runProbe(ctx, args)
	case "connect":
		s.cmdConnect(ctx, args)
	case "known":
		runKnown(ctx, args)
	case "version":
		runVersion(ctx, args)
	default:
		fmt.Printf("未知命令 %q（输入 help 查看）\n", cmd)
	}
}

func (s *replSession) cmdScan(ctx context.Context, args []string) {
	to := 10 * time.Second
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--timeout" {
			if d, err := time.ParseDuration(args[i+1]); err == nil {
				to = d
			}
		}
	}
	fmt.Printf("[*] BLE 扫描 %s ...\n", to)
	devs, err := ac.Scan(ctx, to)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return
	}
	s.lastScan = devs
	fmt.Printf("[*] 扫到 %d 台空调：\n", len(devs))
	marks := knownSet()
	for i, d := range devs {
		tag := ""
		if marks[strings.ToLower(d.UUID)] {
			tag = " ★已知"
		}
		adv := ""
		if d.AdvertisData != nil {
			adv = fmt.Sprintf("%x", d.AdvertisData)
		}
		fmt.Printf("  [%d] SN=%s rssi=%d  %s  adv=%s%s\n", i, d.SN, d.RSSI, d.UUID, adv, tag)
	}
	if len(devs) > 0 {
		fmt.Println("输入 connect <编号> 连接一台设备")
	}
}

func (s *replSession) cmdConnect(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Println("用法: connect <编号|SN|UUID> [--adv HEX]")
		fmt.Println("  编号: 从上一次 scan 结果中选择")
		fmt.Println("  SN:   已知设备序列号")
		fmt.Println("  UUID: macOS 设备标识")
		return
	}
	arg := args[0]

	// Parse optional --adv flag.
	var advFlag string
	for i := 1; i < len(args)-1; i++ {
		if args[i] == "--adv" {
			advFlag = args[i+1]
		}
	}

	// Numeric → index into lastScan.
	if idx, err := strconv.Atoi(arg); err == nil {
		if s.lastScan == nil {
			fmt.Println("尚未扫描，请先执行 scan")
			return
		}
		if idx < 0 || idx >= len(s.lastScan) {
			fmt.Printf("编号 %d 超出范围 (0~%d)，请先 scan\n", idx, len(s.lastScan)-1)
			return
		}
		d := s.lastScan[idx]
		fmt.Printf("连接 [%d] SN=%s %s ...\n", idx, d.SN, d.UUID)
		dev, desc, err := openDevice(ctx, d.UUID, advFlag, ac.DefaultOptions())
		if err != nil {
			fmt.Println("✗", err)
			return
		}
		s.dev = dev
		s.sn = desc.SN
		if s.sn == "" {
			s.sn = desc.UUID
			if len(s.sn) > 8 {
				s.sn = s.sn[:8]
			}
		}
		fmt.Printf("已连接 %s。Tab 补全 / ↑↓ 历史 / disconnect 断开。\n", s.sn)
		printStatus(s.dev.Snapshot(ctx))
		return
	}

	// String → resolve (known SN or UUID).
	fmt.Printf("连接 %s ...\n", arg)
	dev, desc, err := openDevice(ctx, arg, advFlag, ac.DefaultOptions())
	if err != nil {
		fmt.Println("✗", err)
		return
	}
	s.dev = dev
	s.sn = desc.SN
	if s.sn == "" {
		s.sn = desc.UUID
		if len(s.sn) > 8 {
			s.sn = s.sn[:8]
		}
	}
	fmt.Printf("已连接 %s。Tab 补全 / ↑↓ 历史 / disconnect 断开。\n", s.sn)
	printStatus(s.dev.Snapshot(ctx))
}

// ---- connected mode ----

const connectedHelp = `命令（已连接）：
  status              查询并打印实时状态
  on | off            开 / 关机
  temp <16-30>        设定温度（支持 .5）
  mode <auto|cool|dry|heat|fan|smart_dry>
  fan  <low|mid|high|full|auto>
  swing ud|lr|both|off   扫风
  eco on|off          ECO 节能
  strong on|off       强劲
  beep on|off         蜂鸣（本地偏好）
  watch [秒]          监听状态变更（默认 30s 后返回）
  disconnect          断开设备（回到扫描/连接状态）
  help                显示此帮助
  exit | quit         退出程序`

func (s *replSession) execConnected(ctx context.Context, cmd string, args []string) {
	switch cmd {
	case "", "help", "h", "?":
		fmt.Println(connectedHelp)
	case "exit", "quit":
		s.reset()
		os.Exit(0)
	case "disconnect":
		s.reset()
		fmt.Println("已断开。输入 scan 重扫或 connect <编号|SN|UUID> 连接设备。")
	case "status", "s":
		printStatus(s.dev.Snapshot(ctx))
	case "on":
		report(s.dev.Power().Set(ctx, true))
	case "off":
		report(s.dev.Power().Set(ctx, false))
	case "temp":
		if len(args) != 1 {
			fmt.Println("用法: temp <16-30>")
			return
		}
		v, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			fmt.Println("温度需为数字")
			return
		}
		report(s.dev.Temperature().Set(ctx, v))
	case "mode":
		if len(args) != 1 {
			fmt.Println("用法: mode <auto|cool|dry|heat|fan|smart_dry>")
			return
		}
		report(s.dev.Mode().Set(ctx, args[0]))
	case "fan":
		if len(args) != 1 {
			fmt.Println("用法: fan <low|mid|high|full|auto>")
			return
		}
		report(s.dev.Fan().Set(ctx, args[0]))
	case "swing":
		ud, lr := false, false
		switch strings.ToLower(strings.Join(args, "")) {
		case "ud":
			ud = true
		case "lr":
			lr = true
		case "both":
			ud, lr = true, true
		case "off", "":
		default:
			fmt.Println("用法: swing ud|lr|both|off")
			return
		}
		st, err := s.dev.Swing().Set(ctx, ud, lr)
		if err != nil {
			fmt.Printf("✗ %v\n", err)
		} else {
			fmt.Printf("✓ 上下:%v 左右:%v\n", st.UD, st.LR)
		}
	case "eco":
		on, ok := onOff(args)
		if !ok {
			fmt.Println("用法: eco on|off")
			return
		}
		report(s.dev.Eco().Set(ctx, on))
	case "strong":
		on, ok := onOff(args)
		if !ok {
			fmt.Println("用法: strong on|off")
			return
		}
		report(s.dev.Strong().Set(ctx, on))
	case "beep":
		on, ok := onOff(args)
		if !ok {
			fmt.Println("用法: beep on|off")
			return
		}
		s.dev.SetBeep(on)
		fmt.Printf("✓ 蜂鸣=%v\n", on)
	case "watch":
		secs := 30
		if len(args) == 1 {
			if n, e := strconv.Atoi(args[0]); e == nil && n > 0 {
				secs = n
			}
		}
		watchFor(ctx, s.dev, secs)
	default:
		fmt.Printf("未知命令 %q（输入 help 查看）\n", cmd)
	}
}

// ---- completer ----

var modeValues = []string{"auto", "cool", "dry", "heat", "fan", "smart_dry"}
var fanValues = []string{"low", "mid", "high", "full", "auto"}

func toSuggests(vals []string) []prompt.Suggest {
	out := make([]prompt.Suggest, len(vals))
	for i, v := range vals {
		out[i] = prompt.Suggest{Text: v}
	}
	return out
}

var idleSuggests = []prompt.Suggest{
	{Text: "scan", Description: "扫描周围空调（记录结果）"},
	{Text: "connect", Description: "连接到一台设备 connect <编号|SN|UUID>"},
	{Text: "probe", Description: "扫描后逐台握手探查，成功者入库"},
	{Text: "known", Description: "列出已知设备"},
	{Text: "version", Description: "打印版本信息"},
	{Text: "help", Description: "显示帮助"},
	{Text: "exit", Description: "退出程序"},
}

var connectedSuggests = []prompt.Suggest{
	{Text: "status", Description: "查询并打印实时状态"},
	{Text: "on", Description: "开机"},
	{Text: "off", Description: "关机"},
	{Text: "temp", Description: "设定温度 16-30（支持 .5）"},
	{Text: "mode", Description: "模式 auto/cool/dry/heat/fan/smart_dry"},
	{Text: "fan", Description: "风速 low/mid/high/full/auto"},
	{Text: "swing", Description: "扫风 ud|lr|both|off"},
	{Text: "eco", Description: "ECO 节能 on|off"},
	{Text: "strong", Description: "强劲 on|off"},
	{Text: "beep", Description: "蜂鸣 on|off（本地偏好）"},
	{Text: "watch", Description: "监听状态变更 watch [秒]"},
	{Text: "disconnect", Description: "断开设备"},
	{Text: "help", Description: "显示帮助"},
	{Text: "exit", Description: "退出程序"},
}

func (s *replSession) complete(d prompt.Document) []prompt.Suggest {
	if s.dev != nil {
		return s.completeConnected(d)
	}
	return s.completeIdle(d)
}

func (s *replSession) completeIdle(d prompt.Document) []prompt.Suggest {
	fields := strings.Fields(d.TextBeforeCursor())
	word := d.GetWordBeforeCursor()

	// After "connect" → suggest scan indices + known SNs.
	if len(fields) >= 2 && strings.ToLower(fields[0]) == "connect" {
		var args []prompt.Suggest
		if s.lastScan != nil {
			for i, dev := range s.lastScan {
				label := fmt.Sprintf("%d", i)
				desc := dev.SN
				if dev.SN == "" {
					desc = dev.UUID
				}
				args = append(args, prompt.Suggest{Text: label, Description: desc})
			}
		}
		for _, d := range loadKnown() {
			if d.SN != "" {
				args = append(args, prompt.Suggest{Text: d.SN, Description: d.UUID})
			}
		}
		return prompt.FilterHasPrefix(args, word, true)
	}

	if len(fields) == 0 || (len(fields) == 1 && !strings.HasSuffix(d.TextBeforeCursor(), " ")) {
		return prompt.FilterHasPrefix(idleSuggests, word, true)
	}
	return nil
}

func (s *replSession) completeConnected(d prompt.Document) []prompt.Suggest {
	text := d.TextBeforeCursor()
	fields := strings.Fields(text)
	word := d.GetWordBeforeCursor()

	if len(fields) == 0 || (len(fields) == 1 && !strings.HasSuffix(text, " ")) {
		return prompt.FilterHasPrefix(connectedSuggests, word, true)
	}

	// Arg completion for specific commands.
	if len(fields) >= 2 {
		switch strings.ToLower(fields[0]) {
		case "mode":
			return prompt.FilterHasPrefix(toSuggests(modeValues), word, true)
		case "fan":
			return prompt.FilterHasPrefix(toSuggests(fanValues), word, true)
		case "eco", "strong", "beep":
			return prompt.FilterHasPrefix(toSuggests([]string{"on", "off"}), word, true)
		case "swing":
			return prompt.FilterHasPrefix(toSuggests([]string{"ud", "lr", "both", "off"}), word, true)
		}
	}
	return nil
}

// ---- helpers (moved from devshell) ----

// parseCommand splits a line into command + arguments (pure function, testable).
func parseCommand(line string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) == 1 {
		return strings.ToLower(fields[0]), nil
	}
	return strings.ToLower(fields[0]), fields[1:]
}

func onOff(args []string) (bool, bool) {
	if len(args) != 1 {
		return false, false
	}
	switch strings.ToLower(args[0]) {
	case "on", "1", "true":
		return true, true
	case "off", "0", "false":
		return false, true
	}
	return false, false
}

// watchFor prints state changes from device subscriptions for secs seconds,
// then returns.
func watchFor(ctx context.Context, dev ac.IDevice, secs int) {
	fmt.Printf("监听 %ds 状态变更（到时自动返回）…\n", secs)
	deadline := time.After(time.Duration(secs) * time.Second)
	power := dev.Power().Watch()
	mode := dev.Mode().Watch()
	temp := dev.Temperature().Watch()
	fan := dev.Fan().Watch()
	sensor := dev.Sensor().Watch()
	for {
		select {
		case <-deadline:
			fmt.Println("（监听结束）")
			return
		case v := <-power:
			fmt.Printf("  [变更] 电源=%v\n", v)
		case v := <-mode:
			fmt.Printf("  [变更] 模式=%s\n", v)
		case v := <-temp:
			fmt.Printf("  [变更] 温度=%v℃\n", v)
		case v := <-fan:
			fmt.Printf("  [变更] 风速=%s\n", v)
		case v := <-sensor:
			fmt.Printf("  [变更] 室内=%v℃ 室外=%v℃\n", v.In, v.Out)
		}
	}
}

// ---- piped fallback ----

func runPiped(ctx context.Context) {
	exec := func(line string) {
		line = strings.TrimSpace(line)
		switch line {
		case "":
			return
		case "exit", "quit":
			os.Exit(0)
		}
		if err := Dispatch(ctx, strings.Fields(line)); err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
		}
	}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		exec(sc.Text())
	}
}

func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}
