"""Midea AC query and full-state control appliance frames.

Ported from ``internal/ac/appliance.go`` in midea-ble-go.
"""

from dataclasses import dataclass

MODE_AUTO = 1
MODE_COOL = 2
MODE_DRY = 3
MODE_HEAT = 4
MODE_FAN = 5
MODE_SMART_DRY = 6
WIND_LOW = 40
WIND_MID = 60
WIND_HIGH = 80
WIND_FULL = 100
WIND_AUTO = 102
BIZ_TYPE_AC = 0x20


def _checksum(data: bytes) -> int:
    return -sum(data) & 0xFF


def _crc8_854(data: bytes) -> int:
    """Calculate the reflected CRC-8 used by Midea appliance frames."""
    crc = 0
    for value in data:
        crc ^= value
        for _ in range(8):
            crc = (crc >> 1) ^ 0x8C if crc & 1 else crc >> 1
    return crc


@dataclass(slots=True)
class ACState:
    """State fields needed for a safe read-modify-write control command."""

    power: bool = False
    mode: int = MODE_COOL
    target_temperature: float = 26.0
    fan_speed: int = WIND_AUTO
    swing_up_down: bool = False
    swing_left_right: bool = False
    eco: bool = False
    strong: bool = False
    electric_heat: bool = False
    target_temperature_2: float = 26.0


def build_query_frame(order: int, *, sound: bool = False) -> bytes:
    """Build the 24-byte opcode-0x41 status query."""
    frame = bytearray(24)
    frame[0] = 0xAA
    frame[1] = 23
    frame[2] = 0xAC
    frame[9] = 3
    frame[10] = 0x41
    frame[11] = (int(sound) << 6) | 0x21
    frame[13] = 0xFF
    frame[14] = 3
    frame[15] = 0xFF
    frame[17] = 2
    frame[21] = order & 0xFF
    frame[22] = _crc8_854(bytes(frame[10:22]))
    frame[23] = _checksum(bytes(frame[1:23]))
    return bytes(frame)


def build_energy_query_frame() -> bytes:
    """Build the group-4 (power and energy) appliance query.

    Unlike the ordinary 0x41 status query, this group-data form has no message
    id/order byte. Its six-byte body and CRC match Midea's V1 AC protocol; the
    surrounding BLE business/security/connection frames are added separately.
    """
    body = bytes.fromhex("412101440001")
    frame = bytearray(18)
    frame[0] = 0xAA
    frame[1] = len(frame) - 1
    frame[2] = 0xAC
    frame[9] = 3
    frame[10:16] = body
    frame[16] = _crc8_854(body)
    frame[17] = _checksum(bytes(frame[1:17]))
    return bytes(frame)


def _encode_temperature(temperature: float) -> int:
    tenths = int(temperature * 10 + 0.5)
    if not 160 <= tenths <= 300:
        raise ValueError("target temperature must be between 16 and 30 degrees")
    if tenths % 10 not in (0, 5):
        raise ValueError("target temperature must use 0.5-degree steps")
    encoded = (tenths // 10 - 16) & 0x0F
    if tenths % 10 == 5:
        encoded |= 0x10
    return encoded


def build_control_frame(state: ACState, order: int, *, sound: bool = True) -> bytes:
    """Build the 37-byte opcode-0x40 full-state control frame."""
    frame = bytearray(37)
    frame[0] = 0xAA
    frame[1] = 36
    frame[2] = 0xAC
    frame[8] = 2
    frame[9] = 2
    frame[10] = 0x40
    frame[11] = (int(sound) << 6) | (1 << 1) | int(state.power)
    frame[12] = ((state.mode & 7) << 5) | _encode_temperature(
        state.target_temperature
    )
    frame[13] = state.fan_speed & 0x7F
    # No timers. These values match NewACState/BuildControlFrame in Go.
    frame[14] = 0x7F
    frame[15] = 0x7F
    frame[16] = 0xFF
    frame[17] = 0x30
    if state.swing_left_right:
        frame[17] |= 0x03
    if state.swing_up_down:
        frame[17] |= 0x0C
    frame[18] = int(state.strong) << 5
    frame[19] = (int(state.electric_heat) << 3) | (int(state.eco) << 7)
    # Ten default sleep temperatures of 26 C, packed two per byte.
    frame[21:26] = b"\x99" * 5
    frame[27] = 10
    target_2_tenths = int(state.target_temperature_2 * 10 + 1.0)
    frame[28] = (target_2_tenths // 10 - 12) & 0x1F
    frame[29] = 0x80
    frame[34] = order & 0xFF
    frame[35] = _crc8_854(bytes(frame[10:35]))
    frame[36] = _checksum(bytes(frame[1:36]))
    return bytes(frame)
