# Midea BLE protocol port plan

This plan maps the complete Go implementation at repository commit `bace1bb` to
the staged Home Assistant integration. The Go behavior and its tests remain the
source of truth. Python code should reference the relevant Go function in a
short comment and must not copy substantial source blocks verbatim.

## Source-to-target map

| Go source | Python target | Milestone |
|---|---|---:|
| `internal/ble/scanner.go` | `protocol/advertisement.py`, `config_flow.py` | 2 |
| `internal/ble/ble.go` | `bluetooth.py` | 6 |
| `internal/ac/discovery.go` | `config_flow.py`, `device.py` | 2, 6 |
| `internal/ac/transport.go` | `protocol/device.py`, `bluetooth.py` | 5, 6 |
| `internal/proto/crypto.go` | `protocol/crypto.py` | 3 |
| `internal/proto/frame.go` | `protocol/frames.py` | 4 |
| `internal/proto/handshake.go` | `protocol/handshake.py` | 5 |
| `internal/ac/session.go` | `protocol/device.py`, `device.py`, `coordinator.py` | 5–7 |
| `internal/ac/appliance.go` | `protocol/commands.py`, `protocol/status.py`, `protocol/constants.py` | 7–9 |
| `internal/ac/modules.go` | `device.py`, `climate.py`, future `switch.py` | 8–9 |
| `internal/ac/device.go` | `device.py`, `coordinator.py` | 6–9 |
| `internal/ac/interfaces.go` | typed public APIs across `protocol/` and `device.py` | 6–9 |
| `internal/ac/sessiontest/sim.go` | fake transport fixtures under `tests/` | 5–8 |
| `cmd/midea-ble-go/connect.go` | config-entry setup and client lifecycle | 6, 10 |
| `cmd/midea-ble-go/scan.go` | config flow discovery/manual selection | 2 |
| `cmd/midea-ble-go/control.go` | climate command mapping | 8–9 |
| `docs/protocol.md` | module documentation and test expectations | all |
| `docs/architecture.md` | integration layering and ownership | all |

## Component analysis

### Advertisement discovery

- **Purpose:** recognize company ID `0x06A8`, extract a 14-byte ASCII serial, and
  reconstruct the HKDF input `0xAC || SN8 || reverse(payload[19:25])`.
- **Inputs:** BLE manufacturer-data mapping and its company payload.
- **Outputs:** serial, embedded address, raw payload, and 15-byte `advertisData`.
- **State:** upstream scanner retains the longest payload per observed address;
  the pure parser is stateless. Home Assistant's Bluetooth cache supplies the
  newest discovery record.
- **Crypto:** none directly; its output is root-key IKM.
- **Tests:** no dedicated Go unit test. `docs/protocol.md` supplies the example
  payload/advertisData/root key; session fixtures use the same advertisData.
- **Python plan:** strict parsing in `protocol/advertisement.py`; manifest matches
  company ID plus marker and requires `connectable: true`; config flow repeats
  full validation. Prefer the full advertised serial as unique ID. A remote
  connectable HA adapter/proxy is eligible because no local-adapter assumption is
  made.

### BLE transport

- **Purpose:** locate a current BLE target, connect, discover service FFA0,
  write-with-response on FFA1, and receive notifications/indications on FFA2.
- **Inputs:** device address and outgoing complete conn frames.
- **Outputs:** notification byte chunks and disconnect/errors.
- **State:** Go caches address/payload and holds GATT handles plus notification
  callback.
- **Crypto:** none.
- **Tests:** exercised indirectly by AC session tests through an in-memory
  `Transport`; real TinyGo transport has no hardware-free tests.
- **Python plan:** define a minimal async transport boundary below the HA client.
  At connection time call `async_ble_device_from_address(..., connectable=True)`
  to obtain the latest proxy/local target, then use Bleak/HA retry connector.
  Never persist a `BLEDevice`. Serialize writes and unsubscribe/close safely.

### Cryptography

- **Purpose:** derive root/session keys and protect security frames.
- **Inputs:** advertisData; P-256 private32 and peer public `X||Y`; AES key,
  plaintext, and random 8-byte nonce.
- **Outputs:** root key (16), generated P-256 `(private32, public64)`, session key
  (16), or `nonce8 || ciphertext || tag8`.
- **State:** primitives are stateless; ephemeral key material belongs to the
  handshake object and must never enter config entries/logs.
- **Crypto:** HKDF-SHA256 with empty salt and info `midea_bleapp`; P-256 ECDH then
  SHA-256(shared X) truncated to 16; AES-128-CCM with 8-byte nonce/tag and no AAD.
- **Tests:** `TestRootKey` is a fixed Go vector. `TestAESCCMRoundtrip` and
  `TestECDHAgrees` are randomized agreement tests only.
- **Python plan:** use `cryptography` APIs, validate every length, translate
  authentication failures to protocol exceptions, and expose only deterministic
  private-key/nonce injection seams. Add fixed P-256 and CCM vectors and retain
  the exact upstream root-key vector.

### Three-layer framing

- **Purpose:** retain conn (`AA55` boundary/type/checksum), security
  (command/sequence/length), and business (type/length/reserved/checksum) layers.
- **Inputs:** layer type/command, sequence, and body bytes.
- **Outputs:** encoded bytes or decoded typed records.
- **State:** framing is stateless; conn/security sequence counters live in the
  handshake/session state.
- **Crypto:** security frames are encrypted externally; business/conn use
  additive checksums.
- **Tests:** `TestFrameRoundtrip`; appliance golden vectors also exercise biz
  payload framing indirectly.
- **Python plan:** immutable typed decoded frames, exact-length and checksum
  validation, known type validation, and explicit errors for trailing/truncated
  data. This is intentionally stricter than current Go decoders (see ambiguities).

### C1/C2/C3 handshake

- **Purpose:** establish a fresh authenticated encrypted session.
- **Inputs:** advertisData, random six-byte open ID, peer conn frames, sequence and
  randomness sources.
- **Outputs:** C1/C2/C3 conn frames, parsed receive events, and session key.
- **State:** root/session keys, local and peer ephemeral keys, independent conn
  and security counters, and handshake phase.
- **Crypto:** all Milestone 3 primitives. C1/C2/C3 security messages use root key;
  C3 embeds `public64 || AES-CCM(sessionKey, advertisData)`.
- **Tests:** no direct deterministic handshake test; `session_test.go` and
  `sessiontest/sim.go` exercise a complete in-memory exchange.
- **Python plan:** port builders/parser first, then an async phase machine over a
  fake transport. Preserve the unusual retry rules: fresh C1 frames, but reuse
  the exact C2/C3 frame on retransmit to avoid rotating peer key state.

### Session and frame reassembly

- **Purpose:** reassemble notification chunks, route handshake/business replies,
  serialize operations, retry, reconnect, cache state, and publish updates.
- **Inputs:** arbitrary BLE chunks and query/control requests.
- **Outputs:** status snapshots/events and operation failures.
- **State:** receive buffer, phase/generation, pending request, cached mutable AC
  state, subscribers, order counter, transport, and handshake.
- **Crypto:** owns handshake crypto state but delegates primitives.
- **Tests:** `session_test.go` covers handshake, query/control, retry, fragmented
  notifications, reconnect, and watchers; simulator supplies device-side crypto.
- **Python plan:** split pure protocol state/reassembly from HA-aware connection
  acquisition. One `asyncio.Lock` guards session operations. Coordinator/client
  is shared by all entities for one config entry.

### AC commands

- **Purpose:** build 24-byte query and 37-byte full-state control appliance
  frames, including additive checksum and CRC8-854.
- **Inputs:** order/sound plus complete mutable AC state.
- **Outputs:** byte-exact query/control appliance frames.
- **State:** read-modify-write state cache and incrementing order are maintained
  above the encoder.
- **Crypto:** none; frames later enter biz/security/conn layers.
- **Tests:** `TestQueryFrame`, `TestControlFrame26CoolOn`, and
  `TestControlHalfDegree` are reusable golden vectors. `modules_test.go` and
  `wind_test.go` cover validation/mappings.
- **Python plan:** enums and immutable command-state dataclass, literal bit-field
  port, range/half-degree validation, CRC tests, then golden tests for each HA
  operation derived from Go output.

### AC status

- **Purpose:** validate and decode opcode C0 status into user-level state.
- **Inputs:** appliance response bytes.
- **Outputs:** power, mode, target/current/outdoor temperature, fan, swings, ECO,
  strong/turbo, heat, display, tank/error, and retained raw bytes.
- **State:** parser is stateless; session caches result for read-modify-write.
- **Crypto:** none.
- **Tests:** `TestParseStatus` fixed frame plus session/module behavioral tests.
- **Python plan:** a frozen/slotted typed state, defensive bounds checks before
  every indexed field, exact upstream temperature math, raw bytes retained for
  diagnostics, and unknown enum codes represented without crashing.

### Home Assistant mappings

- **Purpose:** one physical device/config entry/coordinator with climate as the
  primary entity and optional ECO/Turbo switches or presets after capability
  behavior is proven.
- **Inputs/outputs:** HA service calls and coordinator state.
- **State:** config data stores address, stable serial, and advertisData only;
  runtime session secrets remain memory-only.
- **Tests:** new mocked config-flow, entity, availability, and reconnect tests.
- **Python plan:** no YAML. Use `ConfigEntryNotReady` for temporary reachability,
  the freshest connectable target from HA Bluetooth, and normal HA enums. Do not
  advertise capabilities that cannot be confirmed from status/device behavior.

## Reusable test vectors

| Vector | Upstream location | Reuse |
|---|---|---|
| advertisData `ac3132…eeff` | `docs/protocol.md`, `sessiontest.Adv` | advertisement + root key |
| root key `3ab82c…cf50` | `internal/proto/vectors_test.go` | HKDF golden test |
| query `aa17ac…a92b` | `internal/ac/appliance_test.go` | command golden test |
| cool/on control `aa24ac…7db9` | same | command golden test |
| half-degree byte `0x5a` | same | temperature encoding |
| C0 response `aa2aac…4c52` | same | status golden test |
| simulator C1/C2/C3 exchange | `internal/ac/sessiontest/sim.go` | fake transport handshake |

The Go repository lacks fixed AES-CCM and ECDH expected bytes. Milestone 3 adds
deterministic Python fixtures using fixed standards-level inputs. Before merging
the handshake milestone, these fixtures should also be emitted/verified by a
small Go test using `CipherMsg` with an injectable nonce (or direct Go CCM) and
fixed P-256 private scalars, then recorded as cross-language provenance.

## Upstream ambiguities and compatibility decisions

1. `DecodeConn` does not verify its checksum, known type, exact total length, or
   trailing bytes. `DecodeSecurity` truncates a declared body longer than the
   available input. Python rejects those malformed inputs. Python validates the
   business frame's structure and length but, like Go, tolerates its additive
   checksum: a real device returned a non-zero value after a successful Python
   handshake. AES-CCM authenticates the containing security frame and the nested
   appliance frame retains its own checksum validation.
2. `ParseStatusFrame` indexes fields before proving the declared/full frame is
   long enough. Python adds minimum/exact-length and checksum validation.
3. Discovery accepts every complete `0x06A8` marker-1 payload; no service UUID or
   explicit product-type discriminator is confirmed upstream. The HA manifest
   therefore uses the strongest known signature (company ID + marker), and the
   config flow performs the complete parser check. A future probe may be needed
   to exclude non-AC Midea products sharing this format.
4. The full 14-byte advertised serial appears stable and is preferred as config
   unique ID, but upstream does not explicitly document global uniqueness. The
   embedded address is kept as discovery metadata, not the HA transport target.
5. Go permits advertisData length 11 in handshake while discovery constructs 15;
   Python discovery requires the complete 25-byte payload/15-byte value.
6. C1 result `1` skips C2/C3 in Go although no cached session key is restored in
   the shown session implementation. This branch needs a captured-device test
   before literal use; otherwise business encryption would have no session key.
7. Status reports fields, not a negotiated capability bitmap. Per-model feature
   discovery and true supported mode/range reporting remain unresolved.
8. Upstream names both `Strong` and misspelled `TubroFuncState`; which bit applies
   across models needs captures before HA exposes one or two Turbo controls.

## Current milestone boundary

The repository now implements discovery/config flow, advertisement parsing,
cryptography, strict framing/reassembly, C1/C2/C3, query/status parsing,
read-modify-write control, Home Assistant-aware BLE dialing, coordinator
lifecycle, and a climate entity. Operations use short-lived transactions to
release local/proxy connection slots. The direct-Bleak probe is a thin wrapper
around the same client used by Home Assistant.

The shared client completed a real handshake/query and verified power on/off
operations with device `00000Q16AC6982`. Remaining work includes diagnostics,
config-flow handshake validation, broader model captures/capability detection,
ECO/Turbo entity design, full Home Assistant fixture coverage, and a real
ESPHome Bluetooth proxy test.
