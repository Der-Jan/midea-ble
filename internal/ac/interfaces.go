package ac

import (
	"context"
)

// ---- 值类型 ----

// SwingState 扫风状态：上下 / 左右。
type SwingState struct {
	UD bool
	LR bool
}

// Ambient 传感器读数：室内 / 室外温度。
type Ambient struct {
	In  float64
	Out float64
}

// ---- 功能模块接口（每个 = Get(Pull) + Set(Write) + Watch(Push)）----

// IPower 电源开关。
type IPower interface {
	Get(ctx context.Context) (bool, error)
	Set(ctx context.Context, on bool) (bool, error)
	Watch() <-chan bool
}

// IMode 运行模式（auto/cool/dry/heat/fan/smart_dry）。
type IMode interface {
	Get(ctx context.Context) (string, error)
	Set(ctx context.Context, mode string) (string, error)
	Watch() <-chan string
}

// ITemperature 设定温度（16~30，步进 0.5）。
type ITemperature interface {
	Get(ctx context.Context) (float64, error)
	Set(ctx context.Context, c float64) (float64, error)
	Watch() <-chan float64
}

// IFan 风速（low/mid/high/full/mute/auto/fixed）。
type IFan interface {
	Get(ctx context.Context) (string, error)
	Set(ctx context.Context, fan string) (string, error)
	Watch() <-chan string
}

// ISwing 扫风（上下/左右）。
type ISwing interface {
	Get(ctx context.Context) (SwingState, error)
	Set(ctx context.Context, ud, lr bool) (SwingState, error)
	Watch() <-chan SwingState
}

// IEco ECO 节能。
type IEco interface {
	Get(ctx context.Context) (bool, error)
	Set(ctx context.Context, on bool) (bool, error)
	Watch() <-chan bool
}

// IStrong 强劲。
type IStrong interface {
	Get(ctx context.Context) (bool, error)
	Set(ctx context.Context, on bool) (bool, error)
	Watch() <-chan bool
}

// ISensor 室内外温度（只读）。
type ISensor interface {
	Get(ctx context.Context) (Ambient, error)
	Watch() <-chan Ambient
}

// IDevice 聚合一台空调的全部能力模块。调用方只依赖本接口。
type IDevice interface {
	Connect(ctx context.Context) error
	Close() error

	Power() IPower
	Mode() IMode
	Temperature() ITemperature
	Fan() IFan
	Swing() ISwing
	Eco() IEco
	Strong() IStrong
	Sensor() ISensor

	// Snapshot 一次性全量读（主动 query）。
	Snapshot(ctx context.Context) (*Status, error)
	// SetBeep 蜂鸣为本地偏好（设备无对应读），影响后续控制帧的 btnSound 位。
	SetBeep(on bool)
}
