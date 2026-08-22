# 美的 / 华凌空调 BLE 协议参考

本文档描述美的系（含华凌子品牌）空调通过 BLE 直连控制所需的链路：BLE 特征、三层帧结构、密钥派生、AES-CCM 加解密、握手时序、业务命令编码。

实现位置：
- 纯协议算法（帧编解码 / 密码学 / 握手状态机）→ `internal/proto/`
- 业务帧构造与解析（控制帧 / 查询帧 / 状态帧）→ `internal/ac/appliance.go`
- 协议驱动引擎（握手相位机 / 重发 / 保活 / pub-sub）→ `internal/ac/session.go`
- 离线一致性向量 → `internal/proto/vectors_test.go`

## 1. BLE 层

| 项 | 值 |
|---|---|
| Primary Service UUID | `0000FFA0-0000-1000-8000-00805F9B34FB` |
| Write Characteristic | `*FFA1*`（按 substring 匹配） |
| Indicate / Notify Characteristic | `*FFA2*` |
| 广播 payload 长度 | 需 **≥ 25 字节**才可重建 advertisData（含 MAC 的完整包） |

广播 payload 中前 15 字节 hex 解 ASCII 即为 14 字符 SN 短码。

### 1.1 链路交互要点

- **FFA1 写特征仅支持 write-with-response**；用 no-response 写会被静默丢弃（macOS 上尤为明显）。
- **握手帧需重发**：设备常忽略一个新连接的前 1~2 个 c1，需用**全新帧（新 seq/nonce）**每 ~1.5s 重发直到应答（配置项 `C1Interval`，见 `DefaultOptions`）。
- **c2/c3 时序敏感**：设备每收一个 c2 会轮换临时 ECDH 密钥对；重复发 c2 会导致用旧公钥算出的 sessionKey 与设备失配 → c3 result=0。应在收到 c2 响应后**立即**发 c3（设备在 c2 后数秒会断连）。代码实现为：c2/c3 阶段用**同一帧**以 `StepInterval`(~1.2s) 间隔重发，不动 nonce/key。
- **业务命令重发要换新 sec_seq**：设备恒丢业务第一帧、且对最后一帧去抖（~550ms 后才回复）。代码策略：首帧后 `BizInterval`(150ms) 快速补发第二帧，之后耐心等 `BizReplyWait`(900ms) 再补，最多 6 次。
- 设备错误以短帧返回：`ff04`=安全层处理失败，`ff05`=计数错误。代码中短帧体（`< 16` 字节）被识别为 `sec_error`。

## 2. 三层帧结构

数据外发顺序：**biz → security 加密 → conn → BLE Write**；接收顺序反向。

### 2.1 连接层（conn）

```
偏移  字段        说明
[0]   0xAA        sync byte 1
[1]   0x55        sync byte 2
[2]   length      length = body.length + 4
[3]   seq         8-bit 序列号，初始随机 1..255，每发 +1
[4]   type        t1=0x01 t2=0x02 t3=0x03
[5..] body
[末]  checksum    = -sum(bytes[2..end-2]) & 0xFF
```

`type` 含义：
- **t1**：连接层握手（"获取版本"，body `[1,0,0,0,0,0,0,0,0,0]`）—— 代码中 `BuildGetVersion()` 已实现但**未被 Session 调用**，实际握手流程直接走 t2。
- **t2**：携带 **rootKey 加密**的 security 帧（c1/c2/c3 阶段）
- **t3**：携带 **sessionKey 加密**的 security 帧（c4 业务阶段）

实现：`EncodeConn` / `DecodeConn`（`internal/proto/frame.go`）。

### 2.2 安全层（security）

```
[0]   cmd        c1=0x01 c2=0x02 c3=0x03 c4=0x04
[1]   seq        8-bit 序列号，初始随机 1..255，每发 +1
[2]   length     body.length
[3..] body
```

**无 checksum**——由 AES-CCM 的 authTag (8B) 提供完整性。

实现：`EncodeSecurity` / `DecodeSecurity`（`internal/proto/frame.go`）。

### 2.3 业务层（biz）

```
[0]   type
[1]   length     = body.length + 4
[2]   预留为 0
[3..] body
[末]  checksum   = -sum(bytes[0..end-2]) & 0xFF
```

实现：`EncodeBiz` / `DecodeBiz`（`internal/proto/frame.go`）。

## 3. 密钥派生

### 3.1 rootKey

```
ikm  = advertisData
salt = []                         ← 空（HMAC-SHA256 下等价 32 字节零盐）
info = "midea_bleapp"
rootKey = HKDF-SHA256(ikm, length=16, salt=[], info="midea_bleapp")
```

注意 salt 为空、`"midea_bleapp"` 是 **info** 而非 salt；搞反则解密失败（ff04）。

**advertisData 的构造**（macOS 不暴露真 MAC，需从广播 payload 重建）：

```
advertisData = 0xAC + SN8(8 字节 ASCII) + MAC(6 字节，逆序)
```

厂商 0x06A8 payload 结构 `[01][SN14][01 03 00 32][MAC6][00]`：SN8 = payload[1:9]，MAC = payload[19:25] 需**逆序**。

示例（构造值）：
```
payload      = 01 3132333435363738414330303031 010300 32 ffeeddccbbaa 00
advertisData = ac 3132333435363738 aabbccddeeff           (15 字节)
rootKey      = 3ab82c346a77b6593d5ebe9f25d3cf50
```

实现：`DeriveRootKey`（`internal/proto/crypto.go`）。

### 3.2 sessionKey（ECDH）

曲线为 **P-256 (secp256r1 / prime256v1)**。

```
priKey = 随机 32 字节 (P-256)
pubKey = P-256 公钥(priKey) → 64 字节 (X||Y，去掉 0x04 前缀)
shared = ECDH(priKey, 0x04||peer_pub64) 的共享点 X 坐标 (32 字节)
sessionKey = SHA-256(shared_x)[0:16]
```

实现：`CreateKeypair` / `DeriveSessionKey`（`internal/proto/crypto.go`）。

## 4. AES-CCM 加解密

```
cipherMsg(key, plaintext):
  nonce  = random_bytes(8)
  ct,tag = AES-128-CCM(key, nonce, plaintext, authTagLength=8)
  output = nonce || ct || tag

decipherMsg(key, blob):
  nonce = blob[0:8]; tag = blob[-8:]; body = blob[8:-8]
  plain = AES-128-CCM_decrypt(key, nonce, body, tag)
```

实现：`CipherMsg` / `DecipherMsg`（`internal/proto/crypto.go`）。

## 5. 握手时序

```
APP                                              空调
 │  BLE Scan + Connect                             │
 │ ───────────────────────────────────────────────→│
 │  rootKey = HKDF(advertisData, "midea_bleapp")    │
 │                                                  │
 │  c1: openId6 (6 字节随机)                         │
 │  outer = conn(t2, cipherMsg(rootKey, c1_frame))  │
 │ ───────────────────────────────────────────────→│
 │←─── c1 响应 result=0（重协商，继续 c2）          │
 │  ── 重协商分支 ──                                │
 │  c2: empty body → conn(t2, cipherMsg(rootKey,c2))│
 │ ───────────────────────────────────────────────→│
 │←─── c2 响应：peerPubKey (64B)                    │
 │                                                  │
 │  生成 P-256 keypair                              │
 │  sessionKey = ECDH(myPri, peerPub)               │
 │                                                  │
 │  c3 body = myPub64(64B) || cipherMsg(sessionKey,advertisData)
 │  outer = conn(t2, cipherMsg(rootKey, c3_frame))  │
 │ ───────────────────────────────────────────────→│
 │←─── c3 响应 result=0 失败 / =1 成功              │
 │  ── 业务阶段 ──                                  │
 │  c4 body = biz_frame → conn(t3, cipherMsg(sk,c4))│
 │ ───────────────────────────────────────────────→│
 │←─── c4 响应（同样 sessionKey 加密）              │
```

要点：
- **c1 预热泵**：用全新帧（新 seq/nonce）每 `C1Interval`(1.5s) 重发，最多 15 次，直到离开 c1 阶段。
- **c2/c3 重发泵**：用同一帧每 `StepInterval`(1.2s) 重发，最多 8 次，避免设备轮换密钥。
- Session engine 代码中**未实现 sessionKey 重用**——每次连接都重新走完整握手流程。

实现：`HandshakeState`（`internal/proto/handshake.go`），`Session.connect()`（`internal/ac/session.go`）。

## 6. 序列号

- 连接层 seq 8-bit，初始随机 1..255，每帧 +1（溢出回绕到 1）。
- 安全层 seq 同理，独立自增。
- 代码中**未实现** seq 单调性校验——接收端不检查 seq 是否乱序/重复。

## 7. 业务命令（biz body）

所有空调控制/查询的 biz frame type 恒为 **32 (0x20)**。`biz.body` 内部是一个**标准美的 appliance 帧**（0xAA 开头）。

### 7.1 appliance 帧结构

```
[0]   0xAA
[1]   帧长 = 总字节数 - 1
[2]   设备类型 = 0xAC（空调）
[3..7] 0
[8]   2（控制/新协议）或 0（标准查询）
[9]   msgType：2=控制(SET) 3=查询(QUERY)
[10]  opcode：0x40 控制 / 0x41 查询 / 0xC0 状态上报
[11..] 数据段
[len-2] crc8_854(数据段从 a[10] 起, 数据段长)
[len-1] makeSum(a[1..len-2])
```

两套校验：
- `crc8_854(arr,n)`：查表 CRC8（表见 `internal/ac/appliance.go` 的 `crc8854Table`），作用于数据段。
- `makeSum(arr,n) = (255 - Σ + 1) & 0xFF`（二补和），作用于整帧尾。

> The Python integration also handles the `0xC1` group-`0x44` power/energy
> response. See [AC power and energy protocol](energy-protocol.md). Other C1
> groups and the `0xB0`/`0xB1` protocol remain unsupported.

### 7.2 控制帧 0x40（共 37 字节，`a[1]=36`）

| 字节 | 位 | 字段 | 编码 |
|---|---|---|---|
| a[11] | bit0 | runStatus 开关机 | 1开0关 |
| a[11] | bit1 | controlSource | 编码时强制 1 |
| a[11] | bit6 | btnSound 蜂鸣 | |
| a[12] | bit5-7 | mode | 1自动 2制冷 3除湿 4制热 5送风 6智能除湿 |
| a[12] | bit0-3 | tempSet 整数 | `(round(temp*10)/10 - 16) & 15`（16~30℃） |
| a[12] | bit4 | 0.5℃ 标志 | `round(temp*10)%10==5` 时置 1 |
| a[13] | bit0-6 | windSpeed | 40低 60中 80高 100强劲 102自动（另有 20 为低档下限值） |
| a[17] | — | 扫风 | 无扫风=0x30；上下=\|0x0C；左右=\|0x03 |
| a[18] | bit5 | strong 强劲 | |
| a[19] | bit3 | elecHeat 电辅热 | |
| a[19] | bit7 | ecoFunc ECO | |
| a[28] | bit0-4 | tempSet2 第二温控 | `(round(t*10+0.5)/10 - 12) & 31` |
| a[34] | — | order 帧序号 | 1..255 自增 |

控制采用「读-改-写」：先 Pull 设备当前状态 → 修改目标字段 → 整帧重编码发出，避免部分覆盖导致遗漏字段。

实现：`BuildControlFrame`（`internal/ac/appliance.go`）。

### 7.3 查询帧 0x41（24 字节）

固定（optCommand=3, queryStat=2, sound=0）：
```
aa17ac00000000000003 41 21 00 ff 03 ff 00 02 00 00 00 <order> <crc8> <sum>
```

实现：`BuildQueryFrame`（`internal/ac/appliance.go`）。

### 7.4 状态回包 0xC0

数据段 `S = frame[10:]`（S[0]=0xC0），常用字段：

| 位置 | 字段 |
|---|---|
| S[1] bit0 / bit7 | runStatus / faultFlag |
| S[2] bit5-7 / bit0-3(+16) / bit4 | mode / tempSet整数 / +0.5 |
| S[3] | windSpeed |
| S[7] | 扫风：<16无；(240&)==48 时 bit0-3 为左右/上下扫风开关 |
| S[8] bit3/5/7 | powerSave/strong/bodySense |
| S[9] bit3/4 | elecHeat/ecoFunc |
| S[11] / S[12] | `(x-50)/2` 室内 / 室外温度（小数在 S[15] 半字节） |
| S[13] bit0-4 | tempSet2 |
| S[16]==38 | 水满 |

实现：`ParseStatusFrame`（`internal/ac/appliance.go`）。
