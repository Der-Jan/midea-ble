package ac

import (
	"context"
	"fmt"
	"sync"
)

// watchMapped 把 Session 的 *Status 流映射为某字段的变更流（去重，缓冲1，满则替换为最新）。
func watchMapped[T comparable](s *Session, mapf func(*Status) T) <-chan T {
	out := make(chan T, 1)
	src, _ := s.Subscribe()
	go func() {
		var last T
		first := true
		for st := range src {
			v := mapf(st)
			if !first && v == last {
				continue
			}
			first, last = false, v
			select {
			case out <- v:
			default:
				select {
				case <-out:
				default:
				}
				select {
				case out <- v:
				default:
				}
			}
		}
		close(out)
	}()
	return out
}

// newWatch 返回一个惰性、单例化的 Watch 闭包（多次调用复用同一订阅）。
func newWatch[T comparable](s *Session, mapf func(*Status) T) func() <-chan T {
	var once sync.Once
	var ch <-chan T
	return func() <-chan T {
		once.Do(func() { ch = watchMapped(s, mapf) })
		return ch
	}
}

// ---- 电源 ----
type powerModule struct {
	s     *Session
	watch func() <-chan bool
}

var _ IPower = (*powerModule)(nil)

func newPowerModule(s *Session) *powerModule {
	return &powerModule{s: s, watch: newWatch(s, func(st *Status) bool { return st.RunStatus == 1 })}
}
func (m *powerModule) Get(ctx context.Context) (bool, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return false, err
	}
	return st.RunStatus == 1, nil
}
func (m *powerModule) Set(ctx context.Context, on bool) (bool, error) {
	st, err := m.s.Control(ctx, func(a *ACState) { a.RunStatus = boolToInt(on) })
	if err != nil {
		return false, err
	}
	return st.RunStatus == 1, nil
}
func (m *powerModule) Watch() <-chan bool { return m.watch() }

// ---- 模式 ----
type modeModule struct {
	s     *Session
	watch func() <-chan string
}

var _ IMode = (*modeModule)(nil)

func newModeModule(s *Session) *modeModule {
	return &modeModule{s: s, watch: newWatch(s, func(st *Status) string { return st.ModeName })}
}
func (m *modeModule) Get(ctx context.Context) (string, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return "", err
	}
	return st.ModeName, nil
}
func (m *modeModule) Set(ctx context.Context, mode string) (string, error) {
	code, ok := ModeByName[mode]
	if !ok {
		return "", fmt.Errorf("未知模式: %s（可选 auto/cool/dry/heat/fan/smart_dry）", mode)
	}
	st, err := m.s.Control(ctx, func(a *ACState) { a.Mode = code })
	if err != nil {
		return "", err
	}
	return st.ModeName, nil
}
func (m *modeModule) Watch() <-chan string { return m.watch() }

// ---- 温度 ----
type temperatureModule struct {
	s     *Session
	watch func() <-chan float64
}

var _ ITemperature = (*temperatureModule)(nil)

func newTemperatureModule(s *Session) *temperatureModule {
	return &temperatureModule{s: s, watch: newWatch(s, func(st *Status) float64 { return st.TempSet })}
}
func (m *temperatureModule) Get(ctx context.Context) (float64, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return 0, err
	}
	return st.TempSet, nil
}
func (m *temperatureModule) Set(ctx context.Context, c float64) (float64, error) {
	if c < 16 || c > 30 {
		return 0, fmt.Errorf("温度需在 16~30 之间: %v", c)
	}
	st, err := m.s.Control(ctx, func(a *ACState) { a.TempSet = c; a.TempSet2 = c })
	if err != nil {
		return 0, err
	}
	return st.TempSet, nil
}
func (m *temperatureModule) Watch() <-chan float64 { return m.watch() }

// ---- 风速 ----
type fanModule struct {
	s     *Session
	watch func() <-chan string
}

var _ IFan = (*fanModule)(nil)

func newFanModule(s *Session) *fanModule {
	return &fanModule{s: s, watch: newWatch(s, func(st *Status) string { return st.WindName })}
}
func (m *fanModule) Get(ctx context.Context) (string, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return "", err
	}
	return st.WindName, nil
}
func (m *fanModule) Set(ctx context.Context, fan string) (string, error) {
	code, ok := WindByName[fan]
	if !ok {
		return "", fmt.Errorf("未知风速: %s（可选 low/mid/high/full/auto）", fan)
	}
	st, err := m.s.Control(ctx, func(a *ACState) { a.WindSpeed = code })
	if err != nil {
		return "", err
	}
	return st.WindName, nil
}
func (m *fanModule) Watch() <-chan string { return m.watch() }

// ---- 扫风 ----
type swingModule struct {
	s     *Session
	watch func() <-chan SwingState
}

var _ ISwing = (*swingModule)(nil)

func swingOf(st *Status) SwingState {
	return SwingState{UD: st.SwingUD == 1, LR: st.SwingLR == 1}
}

func newSwingModule(s *Session) *swingModule {
	return &swingModule{s: s, watch: newWatch(s, swingOf)}
}
func (m *swingModule) Get(ctx context.Context) (SwingState, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return SwingState{}, err
	}
	return swingOf(st), nil
}
func (m *swingModule) Set(ctx context.Context, ud, lr bool) (SwingState, error) {
	st, err := m.s.Control(ctx, func(a *ACState) {
		a.LeftUpDownWind, a.RightUpDownWind = boolToInt(ud), boolToInt(ud)
		a.LeftLeftRightWind, a.RightLeftRightWind = boolToInt(lr), boolToInt(lr)
	})
	if err != nil {
		return SwingState{}, err
	}
	return swingOf(st), nil
}
func (m *swingModule) Watch() <-chan SwingState { return m.watch() }

// ---- ECO ----
type ecoModule struct {
	s     *Session
	watch func() <-chan bool
}

var _ IEco = (*ecoModule)(nil)

func newEcoModule(s *Session) *ecoModule {
	return &ecoModule{s: s, watch: newWatch(s, func(st *Status) bool { return st.EcoFunc == 1 })}
}
func (m *ecoModule) Get(ctx context.Context) (bool, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return false, err
	}
	return st.EcoFunc == 1, nil
}
func (m *ecoModule) Set(ctx context.Context, on bool) (bool, error) {
	st, err := m.s.Control(ctx, func(a *ACState) { a.EcoFunc = boolToInt(on) })
	if err != nil {
		return false, err
	}
	return st.EcoFunc == 1, nil
}
func (m *ecoModule) Watch() <-chan bool { return m.watch() }

// ---- 强劲 ----
type strongModule struct {
	s     *Session
	watch func() <-chan bool
}

var _ IStrong = (*strongModule)(nil)

func newStrongModule(s *Session) *strongModule {
	return &strongModule{s: s, watch: newWatch(s, func(st *Status) bool { return st.Strong == 1 })}
}
func (m *strongModule) Get(ctx context.Context) (bool, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return false, err
	}
	return st.Strong == 1, nil
}
func (m *strongModule) Set(ctx context.Context, on bool) (bool, error) {
	st, err := m.s.Control(ctx, func(a *ACState) { a.Strong = boolToInt(on) })
	if err != nil {
		return false, err
	}
	return st.Strong == 1, nil
}
func (m *strongModule) Watch() <-chan bool { return m.watch() }

// ---- 传感器（只读）----
type sensorModule struct {
	s     *Session
	watch func() <-chan Ambient
}

var _ ISensor = (*sensorModule)(nil)

func ambientOf(st *Status) Ambient { return Ambient{In: st.TempIn, Out: st.TempOut} }

func newSensorModule(s *Session) *sensorModule {
	return &sensorModule{s: s, watch: newWatch(s, ambientOf)}
}
func (m *sensorModule) Get(ctx context.Context) (Ambient, error) {
	st, err := m.s.CachedOrQuery(ctx)
	if err != nil {
		return Ambient{}, err
	}
	return ambientOf(st), nil
}
func (m *sensorModule) Watch() <-chan Ambient { return m.watch() }
