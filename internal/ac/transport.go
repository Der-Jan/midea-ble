// Transport 及 Dialer 是 Session 依赖的最小双向字节通道抽象，与 BLE 平台无关。
// 由本层（ac）定义，由 BLE 层（ble.Conn）结构化实现。生产实现是 ble.Conn，
// 测试实现是 sessiontest 包的内存模拟器。
package ac

import "context"

// Transport 是 Session 依赖的最小双向字节通道（write-with-response 语义 + 通知回调）。
// 生产实现是 ble.Conn（CoreBluetooth/BlueZ）；测试实现是内存模拟器。
type Transport interface {
	// Write 必须以 write-with-response 方式写入（FFA1 仅支持此模式）。
	Write(p []byte) error
	// SetNotify 注册通知回调，参数为设备推来的原始字节片段（可能跨多包，由上层重组）。
	SetNotify(cb func([]byte))
	// Close 断开底层连接。
	Close() error
}

// Dialer 按需建立一个新的 Transport（首连与每次重连各调一次）。
type Dialer func(ctx context.Context) (Transport, error)
