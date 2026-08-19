# Midea BLE for Home Assistant (experimental)

This repository is being developed into a HACS-compatible Home Assistant custom
integration for direct, local control of compatible Midea/Hualing BLE air
conditioners. It includes Bluetooth discovery/config flow, native Python
cryptography and framing, C1/C2/C3 authentication, one-shot connection/session
handling, and a functional climate entity for power, mode, temperature, fan,
and swing control. See
[`docs/protocol-port-plan.md`](docs/protocol-port-plan.md).

Compatibility is limited to devices speaking the protocol implemented by this
repository. It does not imply support for every Midea product.

## Development HACS installation

After these changes are committed and pushed: HACS → Custom repositories → add
this repository as an Integration, install **Midea BLE**, restart Home Assistant,
then use Settings → Devices & services → Add Integration → Midea BLE (or confirm
a Bluetooth discovery notification). Home Assistant 2026.8 or newer is required.

The integration resolves Home Assistant's freshest connectable Bluetooth path
and uses its retry connector, which is the architecture required for supported
ESPHome Bluetooth proxies. A real proxy test is still needed before proxy support
can be claimed as verified.

## Troubleshooting during development

- No discovery: verify the AC is advertising, close enough, and emits complete
  manufacturer data for company ID `0x06A8`.
- Proxy cannot connect: ensure the proxy supports active connections, not only
  passive advertisements.
- Handshake/unknown model: use the development probe below to separate protocol
  authentication failures from Home Assistant setup behavior.
- Debug logging: configure `custom_components.midea_ble` at
  debug level; session keys and private keys will never be logged.

### Development transaction probe

With development dependencies installed, the shared client can be exercised
directly against a BLE address and the 15-byte `advertisData` value:

```bash
python scripts/probe_python.py <BLE_ADDRESS> <ADVERTIS_DATA_HEX>
```

The probe is a thin direct-Bleak wrapper around the same transaction client used
by the integration. By default it performs C1/C2/C3 plus a status query. Passing
`--power-on` or `--power-off` preserves the reported settings, changes only
power, and verifies the returned state. Home Assistant supplies a different
dialer that supports its local adapters and connectable proxies.

---

## Upstream Go protocol reference

[![English](https://img.shields.io/badge/README-English-blue.svg)](README.md)

美的/华凌空调 BLE 直连控制协议参考实现——不经 App、不经云端，从电脑通过蓝牙直接控制你的空调。

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-v0.1.0-lightgrey.svg)](https://github.com/sorinyang/midea-ble-go/tags)

> ⚠️ **免责声明**
>
> 通信协议基于对设备行为的公开研究分析得出，并非官方发布或授权的实现。  
仅供技术学习与合法研究用途，禁止将其用于任何侵犯第三方权益或违反当地法律法规的场景。  
因使用本项目产生的一切后果由使用者自行承担，与项目作者及贡献者无关。 

## 这是什么

- **协议**：描述美的/华凌空调 BLE 通信的完整链路——从蓝牙广播解析、HKDF 密钥派生、P-256 ECDH 密钥协商、AES-128-CCM 加解密，到三层帧编解码与业务命令编码。
- **库**：协议参考实现（`internal/ac`、`internal/proto`、`internal/ble`），按四层架构组织，提供 `IDevice` 门面可直接嵌入你自己的 Go 项目做二次开发。
- **Android 适配**：`mobile` 是面向 gomobile 的薄适配层，将 `internal/ac` 导出为 AAR；Android BluetoothGatt 和 Compose demo 位于独立项目，不混入本仓库的平台目录。
- **CLI**：配套的命令行工具 `midea-ble-go`，支持单次命令执行和 REPL 交互两种模式，用于快速验证协议或日常控制空调。

## 特性

- 支持美的及华凌子品牌空调的 BLE 直连控制（开机/关机/模式/温度/风速/扫风/ECO/强劲）
- 完整实现握手流程（C1→C2→C3）与密钥协商，无需设备预先配对
- 协议层纯算法实现，零外部蓝牙依赖，离线可测（含一致性测试向量）
- 通过 `Transport` 接口与平台解耦，已支持 macOS CoreBluetooth 和 Linux BlueZ
- CLI 提供有状态 REPL 交互模式（连接保持、多轮操作、自动补全）和单次命令模式
- 已知设备自动入库，后续直接用 SN 标识操作，无需每次指定广播数据

## 快速开始

### 安装

```bash
go install github.com/sorinyang/midea-ble-go/cmd/midea-ble-go@latest
```

或从源码编译：

```bash
git clone https://github.com/sorinyang/midea-ble-go.git
cd midea-ble-go
make build
```

### CLI 最小示例

```bash
# 扫描周围空调
midea-ble-go scan

# 探查看一台设备（握手验证），成功自动入库
midea-ble-go probe

# 查询状态
midea-ble-go status 你的设备SN

# 制冷 26℃
midea-ble-go set 你的设备SN --mode cool --temp 26

# 进入 REPL 交互模式
midea-ble-go
```

### 作为库使用

```go
import "github.com/sorinyang/midea-ble-go/internal/ac"

// 扫描设备
devices, _ := ac.Discover(ctx, timeout)

// 连接并握手
dev, _ := ac.OpenDevice(ctx, devices[0], nil)
dev.Connect(ctx)

// 控制和查询
dev.Power().Set(ctx, true)          // 开机
temp, _ := dev.Temperature().Get(ctx)  // 读取当前温度
dev.Mode().Set(ctx, "cool")          // 制冷模式
dev.Fan().Set(ctx, "auto")           // 自动风速

// 订阅状态变更
ch := dev.Watch()
for state := range ch {
    fmt.Printf("电源=%v 模式=%s 温度=%.1f℃\n", state.Run, state.Mode, state.Temp)
}
```

### Android AAR

在本仓库生成协议 AAR：

```bash
gomobile bind -target android -androidapi 23 \\
  -javapkg com.sorinyang.mideable \\
  -o build/midea-mobile.aar ./mobile
```

将生成的文件复制到独立 Android demo 的 `app/libs/midea-mobile.aar`，再由
demo 提供 `mobile.Transport` 的 BluetoothGatt 实现。Android demo 的 Compose
界面、扫描和连接代码不属于本仓库，便于 Android 应用独立演进。

## 基于本协议库的应用

- [Midea-BLE / 美的 BLE（Android）](https://github.com/midea-ble/midea-ble-android#readme)：基于本协议库的独立 Android 应用。

## 文档

- 协议规范（BLE 特征、帧结构、密钥派生、握手时序、业务命令）→ [docs/protocol.md](docs/protocol.md)
- 架构设计（四层分层、关键设计决策、数据流全景）→ [docs/architecture.md](docs/architecture.md)
- CLI 使用说明（命令参考、REPL 模式、已知设备机制）→ [docs/cli.md](docs/cli.md)

## 许可证

本项目代码以 [MIT License](LICENSE) 授权。协议规范文档（`docs/protocol.md`）随代码一同发布，供研究与学习参考。
