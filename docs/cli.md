# midea-ble-go CLI 用法

> 美的 / 华凌空调 BLE 直连控制命令行工具

## 两种运行模式

`midea-ble-go` 支持**双模式**运行，根据是否传入参数自动切换：

### 1. 命令行参数式（单次执行）

传入子命令和参数，执行完毕后立即退出。适合**脚本化、自动化**场景。

```bash
midea-ble-go <子命令> [参数...]
```

特点：
- 一次性执行，不保持连接
- 适合 shell 脚本、cron 定时任务
- 每次执行独立完成「连接→握手→操作→断开」全流程
- 错误通过 `stderr` 输出，退出码非零

### 2. REPL 交互式（有状态多轮对话）

不带任何参数启动，进入交互提示符。适合**手动探索、调试、持续控制**场景。

```bash
midea-ble-go
```

特点：
- **有状态**：连接保持，可在同一会话内反复操作
- **两级模式**：`idle`（未连接）→ `connect` → `connected`（已连接）
- Tab 补全、↑↓ 历史、行编辑
- 保活机制防止蓝牙掉线
- 断线自动重连
- stdin 非终端时（管道/重定向）自动退化为无状态逐行 Dispatch

---

## 子命令一览

| 命令 | 参数 | 说明 |
|------|------|------|
| `scan` | `[--timeout DUR]` | 扫描周围空调，列出 SN / UUID / RSSI |
| `probe` | `[--timeout DUR]` | 扫描后逐台握手探查，成功者自动入库 |
| `status` | `<dev> [--adv HEX]` | 握手 + 查询设备实时状态 |
| `on` | `<dev> [--adv HEX]` | 开机 |
| `off` | `<dev> [--adv HEX]` | 关机 |
| `set` | `<dev> [flags...]` | 设定模式/温度/风速/扫风等（隐含开机） |
| `handshake` | `<dev> [--adv HEX]` | 仅验证握手，不查询不控制 |
| `known` | — | 列出已知设备（`~/.midea-ble-go/known_devices.json`） |
| `version` | — | 打印版本号 / commit / 构建时间 |
| `help` | — | 显示帮助 |

### `<dev>` 设备标识

`<dev>` 可以是以下两种形式：

1. **UUID**：macOS 蓝牙设备标识（隐私 UUID），如 `C111157E-FC67-...-690C955E`，通过 `scan` 获取
2. **SN**：已知设备的 14 字符短标识（如 `12345678AC0001`），需该设备已通过握手入库

当使用 SN 时，工具自动从已知设备注册表获取缓存的 `advertisData` 和 UUID，无需手动指定 `--adv`。

> **注意**：BLE 广播中的「SN」是 14 字符短标识（厂商/批次/单元码 + MAC 派生），**非机身完整出厂序列号**。

---

## `set` 命令参数详解

`set` 是最核心的控制命令，所有参数均为可选（至少指定一个）：

```
midea-ble-go set <dev> [--adv HEX]
    [--mode auto|cool|dry|heat|fan|smart_dry]
    [--temp 16-30]
    [--fan low|mid|high|full|mute|auto|fixed]
    [--swing-ud]
    [--swing-lr]
    [--eco]
    [--strong]
    [--no-beep]
```

| 参数 | 类型 | 说明 |
|------|------|------|
| `--adv` | hex string | 手动指定 advertisData（hex 编码），一般无需手动提供 |
| `--mode` | string | 运行模式：`auto`（自动）/ `cool`（制冷）/ `dry`（除湿）/ `heat`（制热）/ `fan`（送风）/ `smart_dry`（智能除湿） |
| `--temp` | float | 设定温度，范围 16~30℃，支持 0.5℃ 步进（如 `25.5`） |
| `--fan` | string | 风速：`low`（低）/ `mid`（中）/ `high`（高）/ `full`（强劲）/ `mute`（静音）/ `auto`（自动）/ `fixed`（固定） |
| `--swing-ud` | bool | 开启上下扫风 |
| `--swing-lr` | bool | 开启左右扫风 |
| `--eco` | bool | 开启 ECO 节能模式 |
| `--strong` | bool | 开启强劲模式 |
| `--no-beep` | bool | 静音操作（抑制蜂鸣声，本地偏好，设备无对应查询） |

> `set` 命令**隐含开机**（自动调用 `Power().Set(true)`），无需单独 `on`。

**示例：**

```bash
# 制冷 26℃，自动风速
midea-ble-go set 12345678AC0001 --mode cool --temp 26 --fan auto

# 制热 24℃，静音风速 + 上下扫风
midea-ble-go set 12345678AC0001 --mode heat --temp 24 --fan mute --swing-ud

# 仅调温度（保持当前模式和风速）
midea-ble-go set 12345678AC0001 --temp 22

# 静音操作（不发出蜂鸣声）
midea-ble-go set 12345678AC0001 --temp 26 --no-beep

# 手动指定 advertisData（广播数据不完整时）
midea-ble-go set C111157E-FC67-4E5E-8C4F-690C955E --adv ac3132333435363738aabbccddeeff --mode cool --temp 26
```

---

## 其他命令示例

### scan — 扫描设备

```bash
midea-ble-go scan                    # 默认扫描 10s
midea-ble-go scan --timeout 5s       # 扫描 5 秒
midea-ble-go scan --timeout 30s      # 扫描 30 秒
```

输出示例：
```
[*] BLE 扫描 10s ...
[*] 扫到 2 台空调：
  [0] SN=12345678AC0001 rssi=-51  C111157E-FC67-4E5E-8C4F-690C955E  adv=ac3132333435363738aabbccddeeff
  [1] SN=12345679AC0002 rssi=-68  9D2A0B14-77EE-459B-9D72-4C1A0033  adv=ac3132333435363739ccddeeff0011
```

### probe — 探查设备

扫描后对每台设备逐一尝试握手，成功者自动记入已知设备注册表：

```bash
midea-ble-go probe                   # 默认扫描 12s，每台握手最多 25s
midea-ble-go probe --timeout 15s     # 扫描 15 秒
```

### status — 查询状态

```bash
midea-ble-go status 12345678AC0001   # 用 SN 直接查询
midea-ble-go status C11115...        # 用 UUID 查询
```

输出示例：
```
电源:开  模式:cool  温度:26℃  风速:auto  室内:26.7℃  室外:29.3℃  ECO:0  强劲:0  故障:0
```

### on / off — 开关机

```bash
midea-ble-go on 12345678AC0001
midea-ble-go off 12345678AC0001
```

### handshake — 验证握手

```bash
midea-ble-go handshake 12345678AC0001
# 输出: ✓ 握手成功
```

### known — 已知设备

```bash
midea-ble-go known
```

输出列出 `~/.midea-ble-go/known_devices.json` 中所有已验证设备。

### version — 版本

```bash
midea-ble-go version
# 输出: midea-ble-go v1.0.0 (commit abc1234, built 2026-07-01T12:00:00Z)
```

---

## REPL 交互模式

### idle 状态（未连接设备）

```
midea-ble-go> help
命令（idle）：
  scan [--timeout DUR]       扫描周围空调（记录结果供 connect 用）
  connect <编号|SN|UUID>     连接到一台设备
  probe [--timeout DUR]      扫描后逐台握手探查，成功者入库
  known                      列出已知设备
  version                    打印版本信息
  help                       显示此帮助
  exit | quit                退出程序
```

典型流程：
```
midea-ble-go> scan
[*] 扫到 2 台空调：
  [0] SN=12345678AC0001 rssi=-51  C111157E-...  ★已知
  [1] SN=12345679AC0002 rssi=-68  9D2A0B14-...
输入 connect <编号> 连接一台设备

midea-ble-go> connect 0
连接 [0] SN=12345678AC0001 ...
已连接 12345678AC0001。Tab 补全 / ↑↓ 历史 / disconnect 断开。
```

### connected 状态（已连接设备）

```
12345678AC0001> help
命令（已连接）：
  status              查询并打印实时状态
  on | off            开 / 关机
  temp <16-30>        设定温度（支持 .5）
  mode <auto|cool|dry|heat|fan|smart_dry>
  fan  <low|mid|high|full|mute|auto|fixed>
  swing ud|lr|both|off   扫风
  eco on|off          ECO 节能
  strong on|off       强劲
  beep on|off         蜂鸣（本地偏好）
  watch [秒]          监听状态变更（默认 30s 后返回）
  disconnect          断开设备（回到扫描/连接状态）
  help                显示此帮助
  exit | quit         退出程序
```

典型操作：
```
12345678AC0001> status
电源:开  模式:cool  温度:26℃  风速:auto  室内:26.7℃  室外:29.3℃

12345678AC0001> temp 24
✓ 24

12345678AC0001> fan mute
✓ mute

12345678AC0001> swing ud
✓ 上下:true 左右:false

12345678AC0001> eco on
✓ true

12345678AC0001> watch 10
监听 10s 状态变更（到时自动返回）…
  [变更] 温度=25.5℃
  [变更] 室内=25.7℃ 室外=28.1℃
（监听结束）

12345678AC0001> disconnect
已断开。输入 scan 重扫或 connect <编号|SN|UUID> 连接设备。
```

### watch — 实时监听

`watch [秒]` 在已连接状态下订阅设备状态变更，限时运行到时自动返回（默认 30 秒）。可监听的变更包括：电源、模式、温度、风速、室内外温度传感器。

---

## 已知设备机制

握手成功的设备会自动记录到 `~/.midea-ble-go/known_devices.json`：

```json
[
  {
    "sn": "12345678AC0001",
    "uuid": "C111157E-FC67-4E5E-8C4F-690C955E",
    "advertisData": "ac3132333435363738aabbccddeeff",
    "handshake": true,
    "verifiedAt": "2026-07-01T15:17:00+08:00"
  }
]
```

入库后即可**直接用 SN 替代 UUID** 操作设备（自动复用缓存的 `advertisData`），免去每次手动指定 `--adv`。

入库方式：
1. `probe` 命令 — 扫描 + 批量握手探查
2. 任何成功握手的操作（`status` / `on` / `off` / `set` / `handshake`）
