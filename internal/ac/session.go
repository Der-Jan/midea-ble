package ac

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sorinyang/midea-ble-go/internal/proto"
)

var errDisconnected = errors.New("未连接")

var debugOn = os.Getenv("HLCTL_DEBUG") != ""

func debugf(format string, a ...any) {
	if debugOn {
		fmt.Fprintf(os.Stderr, "[debug] "+format+"\n", a...)
	}
}

// Options 控制握手/重发/保活/超时时序。
type Options struct {
	C1Interval   time.Duration // c1 预热重发间隔
	StepInterval time.Duration // c2/c3 重发间隔
	BizInterval  time.Duration // 业务首帧后的快速补发间隔（设备恒丢第一帧）
	BizReplyWait time.Duration // 补发后等待回复的间隔（设备对最后一帧去抖，约 550ms 后回复）
	Keepalive    time.Duration // 保活查询间隔
	OpTimeout    time.Duration // 单次操作总超时
	MaxReconnect int           // 重连尝试次数
}

// DefaultOptions 返回默认时序参数。
func DefaultOptions() Options {
	return Options{
		C1Interval:   1500 * time.Millisecond,
		StepInterval: 1200 * time.Millisecond,
		// 设备可能丢弃业务首帧，并且 Android BLE 指示通知会排队，
		// 因此首帧快速补发，后续等待足够长的去抖和传输时间。
		BizInterval:  100 * time.Millisecond,
		BizReplyWait: 5 * time.Second,
		// 设备会主动推送状态，不需要额外的查询保活。
		Keepalive:    0,
		OpTimeout:    30 * time.Second,
		MaxReconnect: 3,
	}
}

// Session 是协议驱动引擎：在可注入的 Transport 上完成握手、查询、控制、保活、重连，
// 并把设备状态帧以 pub/sub 广播给订阅者。各功能模块共享一个 Session。
type Session struct {
	dial         Dialer
	advertisData []byte
	openID6      []byte
	opt          Options

	mu      sync.Mutex
	t       Transport
	hs      *proto.HandshakeState
	ac      *ACState
	cached  *Status
	phase   string // "" | c1 | c2 | c3 | biz | closed
	gen     int    // 每次 connect 自增，作废上一代的重发泵
	beep    bool
	recvBuf []byte

	hsResult chan error
	pending  chan *Status

	subs    map[int]chan *Status
	nextSub int

	opMu          sync.Mutex // 串行化 Handshake/Query/Control/keepalive
	keepaliveStop chan struct{}
	closed        bool
	order         int
}

// NewSession 创建会话（尚未连接）。dial 用于按需建立 Transport。
func NewSession(dial Dialer, advertisData, openID6 []byte, opt Options) (*Session, error) {
	if dial == nil {
		return nil, errors.New("dialer 不能为空")
	}
	if len(advertisData) < 11 {
		return nil, fmt.Errorf("advertisData 太短: %d", len(advertisData))
	}
	if len(openID6) != 6 {
		return nil, errors.New("openID6 必须 6 字节")
	}
	// 会话需要长期持有这两段数据。特别是 gomobile 调用中，传入的
	// Java byte[] 可能在桥接函数返回后解除固定，不能直接保存底层切片。
	adCopy := append([]byte(nil), advertisData...)
	idCopy := append([]byte(nil), openID6...)
	return &Session{
		dial: dial, advertisData: adCopy, openID6: idCopy,
		opt: opt, beep: true, subs: map[int]chan *Status{},
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- 帧重组与分发 ----

func indexAA55(b []byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == 0xAA && b[i+1] == 0x55 {
			return i
		}
	}
	return -1
}

func (s *Session) onRaw(b []byte) {
	s.mu.Lock()
	s.recvBuf = append(s.recvBuf, b...)
	var frames [][]byte
	for {
		i := indexAA55(s.recvBuf)
		if i < 0 {
			s.recvBuf = s.recvBuf[:0]
			break
		}
		if i > 0 {
			s.recvBuf = s.recvBuf[i:]
		}
		if len(s.recvBuf) < 3 {
			break
		}
		// LEN 表示从 LEN 字段开始到校验和的总长度，因此完整帧为
		// 2 字节帧头加 LEN 字节。多加一个长度字节会截断所有回包。
		total := 2 + int(s.recvBuf[2])
		if len(s.recvBuf) < total {
			break
		}
		frames = append(frames, append([]byte(nil), s.recvBuf[:total]...))
		s.recvBuf = s.recvBuf[total:]
	}
	hs := s.hs
	s.mu.Unlock()
	for _, f := range frames {
		s.handleFrame(hs, f)
	}
}

func (s *Session) handleFrame(hs *proto.HandshakeState, frame []byte) {
	if hs == nil {
		return
	}
	info, err := hs.OnRecv(frame)
	if err != nil {
		debugf("← 解析失败 @%s len=%d", time.Now().Format("05.000"), len(frame))
		return
	}
	debugf("← %s @%s", info.Kind, time.Now().Format("05.000"))
	switch info.Kind {
	case "sec_error":
		return
	case "c1":
		s.mu.Lock()
		if s.phase != "c1" {
			s.mu.Unlock()
			return
		}
		if info.Result == 1 {
			s.phase = "biz"
			s.mu.Unlock()
			s.signalHS(nil)
			return
		}
		s.phase = "c2"
		gen := s.gen
		c2, _ := hs.BuildC2()
		s.mu.Unlock()
		go s.pumpStep(gen, "c2", c2)
	case "c2":
		s.mu.Lock()
		if s.phase != "c2" {
			s.mu.Unlock()
			return
		}
		hs.PeerPub64 = info.PeerPub64
		pri, pub, err := proto.CreateKeypair()
		if err != nil {
			s.phase = "closed"
			s.mu.Unlock()
			s.signalHS(err)
			return
		}
		hs.MyPri, hs.MyPub64 = pri, pub
		sk, err := proto.DeriveSessionKey(pri, info.PeerPub64)
		if err != nil {
			s.phase = "closed"
			s.mu.Unlock()
			s.signalHS(err)
			return
		}
		hs.SessionKey = sk
		s.phase = "c3"
		gen := s.gen
		c3, err := hs.BuildC3()
		s.mu.Unlock()
		if err != nil {
			s.signalHS(err)
			return
		}
		go s.pumpStep(gen, "c3", c3)
	case "c3":
		s.mu.Lock()
		if s.phase != "c3" {
			s.mu.Unlock()
			return
		}
		if info.Result == 1 {
			s.phase = "biz"
			s.mu.Unlock()
			s.signalHS(nil)
		} else {
			s.phase = "closed"
			s.mu.Unlock()
			s.signalHS(errors.New("c3 result=0（sessionKey/advertisData 校验未过）"))
		}
	case "biz":
		st, err := ParseStatusFrame(info.BizBody)
		if err != nil {
			return
		}
		s.mu.Lock()
		s.cached = st
		if s.ac == nil {
			s.ac = NewACState()
		}
		s.ac.ApplyStatus(st)
		pending := s.pending
		subs := make([]chan *Status, 0, len(s.subs))
		for _, c := range s.subs {
			subs = append(subs, c)
		}
		s.mu.Unlock()
		if pending != nil {
			select {
			case pending <- st:
			default:
			}
		}
		for _, c := range subs {
			select {
			case c <- st:
			default:
			}
		}
	}
}

func (s *Session) signalHS(err error) {
	s.mu.Lock()
	ch := s.hsResult
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- err:
		default:
		}
	}
}

func (s *Session) phaseGen() (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase, s.gen
}

// c1 预热泵：用全新帧（新 seq/nonce）每 C1Interval 重发，直到离开 c1 阶段或换代。
func (s *Session) pumpC1(gen int) {
	for i := 0; i < 15; i++ {
		p, g := s.phaseGen()
		if g != gen || p != "c1" {
			return
		}
		s.mu.Lock()
		hs := s.hs
		s.mu.Unlock()
		if hs == nil {
			return
		}
		if f, err := hs.BuildC1(); err == nil {
			s.write(f)
		}
		time.Sleep(s.opt.C1Interval)
	}
}

// c2/c3 泵：重发同一帧（避免设备轮换密钥），直到离开该阶段或换代。
func (s *Session) pumpStep(gen int, phase string, frame []byte) {
	for i := 0; i < 8; i++ {
		p, g := s.phaseGen()
		if g != gen || p != phase {
			return
		}
		s.write(frame)
		time.Sleep(s.opt.StepInterval)
	}
}

func (s *Session) write(frame []byte) {
	s.mu.Lock()
	t := s.t
	s.mu.Unlock()
	if t != nil {
		_ = t.Write(frame)
	}
}

// ---- 连接 / 重连 ----

// Handshake 建立连接并完成握手，随后启动保活。
func (s *Session) Handshake(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.connect(ctx); err != nil {
		return err
	}
	s.startKeepalive()
	return nil
}

// connect 假定调用方持有 opMu。
func (s *Session) connect(ctx context.Context) error {
	t, err := s.dial(ctx)
	if err != nil {
		return err
	}
	hs, err := proto.NewHandshakeState(s.advertisData, s.openID6)
	if err != nil {
		_ = t.Close()
		return err
	}
	s.mu.Lock()
	if s.t != nil {
		_ = s.t.Close()
	}
	s.t = t
	s.hs = hs
	s.ac = NewACState()
	s.ac.BtnSound = boolToInt(s.beep)
	s.phase = "c1"
	s.gen++
	gen := s.gen
	s.recvBuf = nil
	s.hsResult = make(chan error, 1)
	s.mu.Unlock()

	t.SetNotify(s.onRaw)
	time.Sleep(300 * time.Millisecond)
	go s.pumpC1(gen)

	select {
	case err := <-s.hsResult:
		if err != nil {
			s.setPhase("closed")
			return err
		}
	case <-time.After(s.opt.OpTimeout):
		s.setPhase("closed")
		return errors.New("握手超时")
	case <-ctx.Done():
		s.setPhase("closed")
		return ctx.Err()
	}
	if !s.Connected() {
		return errors.New("握手未达 biz 阶段")
	}
	// 初始全量读，填充缓存（失败不致命）
	_, _ = s.doBiz(ctx, BuildQueryFrame(1, 0))
	return nil
}

func (s *Session) setPhase(p string) {
	s.mu.Lock()
	s.phase = p
	s.mu.Unlock()
}

// Connected 报告会话是否已完成握手、进入业务阶段（sessionKey 已就绪）。
func (s *Session) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase == "biz" && s.hs != nil && s.hs.SessionKey != nil
}

func (s *Session) ensureConnected(ctx context.Context) error {
	if s.Connected() {
		return nil
	}
	var err error
	for i := 0; i < s.opt.MaxReconnect; i++ {
		fmt.Println("[*] 重连中…")
		if err = s.connect(ctx); err == nil {
			return nil
		}
	}
	return fmt.Errorf("重连失败: %w", err)
}

// ---- 业务收发 ----

// doBiz 发送一条 appliance 业务帧并等待 0xC0 回包；按 BizInterval 用新 sec_seq 重发。
// 假定调用方持有 opMu，且已连接。
func (s *Session) doBiz(ctx context.Context, applianceBiz []byte) (*Status, error) {
	s.mu.Lock()
	if s.phase != "biz" || s.hs == nil {
		s.mu.Unlock()
		return nil, errDisconnected
	}
	hs := s.hs
	t := s.t
	pend := make(chan *Status, 1)
	s.pending = pend
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.pending = nil
		s.mu.Unlock()
	}()

	start := time.Now()
	deadline := time.After(s.opt.OpTimeout)
	frame, err := hs.BuildBiz(BizTypeAC, applianceBiz)
	if err != nil {
		return nil, err
	}
	debugf("→ 发送#1 @%s", time.Now().Format("05.000"))
	if err := t.Write(frame); err != nil {
		s.setPhase("closed")
		return nil, fmt.Errorf("写失败（链路断）: %w", err)
	}

	// 设备通常丢弃首帧，并对最后一帧做约 550ms 去抖；Android BLE
	// 的 indication 还可能在链路上排队，所以每次重试给出充足等待时间。
	const retryWait = 3 * time.Second
	for i := 1; i <= 3; i++ {
		select {
		case st := <-pend:
			debugf("doBiz: %d 帧, 耗时 %v", i, time.Since(start))
			return st, nil
		case <-time.After(retryWait):
		case <-deadline:
			return nil, errors.New("业务超时（无回包）")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		frame, err = hs.BuildBiz(BizTypeAC, applianceBiz)
		if err != nil {
			return nil, err
		}
		debugf("→ 发送#%d @%s", i+1, time.Now().Format("05.000"))
		if err := t.Write(frame); err != nil {
			s.setPhase("closed")
			return nil, fmt.Errorf("写失败（链路断）: %w", err)
		}
	}
	return nil, errors.New("业务无回包")
}

// fireBiz 发送单帧业务命令，不等待回复。用于不应阻塞用户操作的保活。
func (s *Session) fireBiz(applianceBiz []byte) error {
	s.mu.Lock()
	if s.phase != "biz" || s.hs == nil {
		s.mu.Unlock()
		return errDisconnected
	}
	hs := s.hs
	t := s.t
	s.mu.Unlock()
	frame, err := hs.BuildBiz(BizTypeAC, applianceBiz)
	if err != nil {
		return err
	}
	return t.Write(frame)
}

// Query 主动查询并刷新缓存。
func (s *Session) Query(ctx context.Context) (*Status, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.ensureConnected(ctx); err != nil {
		return nil, err
	}
	return s.doBiz(ctx, BuildQueryFrame(s.nextOrder(), 0))
}

// Control 读-改-写：对缓存 ACState 应用 mutate 后下发整帧，返回变更后状态。
func (s *Session) Control(ctx context.Context, mutate func(*ACState)) (*Status, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.ensureConnected(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.ac == nil {
		s.ac = NewACState()
	}
	mutate(s.ac)
	s.ac.Order = s.orderLocked()
	frame, err := BuildControlFrame(s.ac)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return s.doBiz(ctx, frame)
}

func (s *Session) nextOrder() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.orderLocked()
}

func (s *Session) orderLocked() int {
	s.order = s.order%255 + 1
	return s.order
}

// ---- pub/sub / 缓存 / 偏好 ----

// Subscribe 注册一个状态订阅，返回通道与取消函数。
func (s *Session) Subscribe() (<-chan *Status, func()) {
	ch := make(chan *Status, 4)
	s.mu.Lock()
	id := s.nextSub
	s.nextSub++
	if s.subs == nil {
		s.subs = map[int]chan *Status{}
	}
	s.subs[id] = ch
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		if _, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(ch)
		}
		s.mu.Unlock()
	}
}

// Cached 返回最近一次状态（可能为 nil）。
func (s *Session) Cached() *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cached
}

// CachedOrQuery 有缓存返回缓存，否则主动查询。
func (s *Session) CachedOrQuery(ctx context.Context) (*Status, error) {
	if st := s.Cached(); st != nil {
		return st, nil
	}
	return s.Query(ctx)
}

// SetBeep 设置本地蜂鸣偏好（影响后续控制帧）。
func (s *Session) SetBeep(on bool) {
	s.mu.Lock()
	s.beep = on
	if s.ac != nil {
		s.ac.BtnSound = boolToInt(on)
	}
	s.mu.Unlock()
}

// ---- 保活 / 关闭 ----

func (s *Session) startKeepalive() {
	if s.opt.Keepalive <= 0 {
		return
	}
	s.mu.Lock()
	if s.keepaliveStop != nil {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.keepaliveStop = stop
	s.mu.Unlock()
	go func() {
		ticker := time.NewTicker(s.opt.Keepalive)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				// 保活只需发出查询，不等待设备回包；否则会与用户操作
				// 争用 opMu，并把设备主动推送的状态误当作查询响应。
				if !s.opMu.TryLock() {
					continue
				}
				if s.Connected() {
					_ = s.fireBiz(BuildQueryFrame(s.nextOrder(), 0))
				}
				s.opMu.Unlock()
			}
		}
	}()
}

// Close 停止保活并断开连接。
func (s *Session) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.phase = "closed"
	stop := s.keepaliveStop
	t := s.t
	s.t = nil
	s.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if t != nil {
		return t.Close()
	}
	return nil
}
