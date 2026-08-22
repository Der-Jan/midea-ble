"""Midea AC capability and optional-property appliance packets."""

from __future__ import annotations

from dataclasses import dataclass
from enum import IntEnum

from .commands import _checksum, _crc8_854
from .exceptions import MideaBleFrameError


class ACProperty(IntEnum):
    """Prioritized B0/B1 AC property tags."""

    WIND_UD_ANGLE = 0x0009
    WIND_LR_ANGLE = 0x000A
    INDOOR_HUMIDITY = 0x0015
    SCREEN_DISPLAY = 0x0017
    SELF_CLEAN = 0x0039
    ERROR_CODE = 0x003F
    RATE_SELECT = 0x0048
    OUTDOOR_SILENT = 0x00CD


class B5Capability(IntEnum):
    """Relevant B5 capability tags."""

    MODE = 0x0214
    SWING = 0x0215
    ELECTRICITY = 0x0216
    DISPLAY = 0x0224
    HUMIDITY = 0x021F
    SELF_CLEAN = 0x0039


@dataclass(frozen=True, slots=True)
class ACOptionalState:
    """Values returned by independently verified optional-property queries."""

    values: dict[ACProperty, bytes]
    b5_values: dict[int, bytes]

    def value(self, prop: ACProperty) -> int | None:
        data = self.values.get(prop)
        return data[0] if data else None


def _build_frame(body_without_crc: bytes, message_type: int, order: int) -> bytes:
    body = bytearray(body_without_crc)
    body.append(order & 0xFF)
    body.append(_crc8_854(bytes(body)))
    frame = bytearray(10 + len(body) + 1)
    frame[0] = 0xAA
    frame[1] = len(frame) - 1
    frame[2] = 0xAC
    # PortaSplit's B0/B1/B5 "new protocol" is appliance protocol version 8.
    # This byte remains inside the AA appliance packet; it is unrelated to BLE.
    frame[8] = 8
    frame[9] = message_type
    frame[10:-1] = body
    frame[-1] = _checksum(bytes(frame[1:-1]))
    return bytes(frame)


def build_capability_query_frame(order: int, *, additional: bool = False) -> bytes:
    """Build the first or additional B5 capability query."""
    payload = b"\xb5\x01\x01\x01" if additional else b"\xb5\x01\x00"
    return _build_frame(payload, 0x03, order)


def build_property_query_frame(properties: tuple[ACProperty, ...], order: int) -> bytes:
    """Build a B1 query for one or more appliance properties."""
    if not properties or len(properties) > 0xFF:
        raise ValueError("property query must contain 1..255 tags")
    body = bytearray((0xB1, len(properties)))
    for prop in properties:
        body.extend((int(prop) & 0xFF, int(prop) >> 8))
    return _build_frame(bytes(body), 0x03, order)


def build_property_set_frame(prop: ACProperty, value: int, order: int) -> bytes:
    """Build a single-property B0 absolute control packet."""
    if not 0 <= value <= 0xFF:
        raise ValueError("property value must fit in one byte")
    # PortaSplit expects the current prompt-tone property alongside B0 writes,
    # matching midea-local's MessageNewProtocolSet construction.
    body = bytes(
        (
            0xB0,
            0x02,
            int(prop) & 0xFF,
            int(prop) >> 8,
            1,
            value,
            0x1A,
            0,
            1,
            1,
        )
    )
    return _build_frame(body, 0x02, order)


def build_display_toggle_frame(order: int, *, sound: bool = False) -> bytes:
    """Build the legacy 0x41 display-toggle appliance command."""
    payload = bytearray(20)
    payload[0] = 0x41
    payload[1] = 0x02 | (0x40 if sound else 0)
    payload[3] = 0xFF
    payload[4] = 0x02
    payload[6] = 0x02
    return _build_frame(bytes(payload), 0x03, order)


def _validate_appliance_frame(frame: bytes, body_types: tuple[int, ...]) -> bytes:
    if len(frame) < 14 or frame[0] != 0xAA:
        raise MideaBleFrameError("optional-property appliance frame is invalid")
    if len(frame) != frame[1] + 1:
        raise MideaBleFrameError("optional-property appliance length mismatch")
    if sum(frame[1:]) & 0xFF:
        raise MideaBleFrameError("optional-property appliance checksum mismatch")
    if frame[10] not in body_types:
        raise MideaBleFrameError(f"unexpected body type 0x{frame[10]:02x}")
    return frame[10:-1]


def parse_tlv_response(frame: bytes) -> tuple[int, dict[int, bytes]]:
    """Parse B0/B1/B5 appliance TLVs, excluding message-id/CRC trailers."""
    body = _validate_appliance_frame(frame, (0xB0, 0xB1, 0xB5))
    body_type = body[0]
    count = body[1]
    position = 2
    values: dict[int, bytes] = {}
    trailer = 2
    for _ in range(count):
        header_length = 3 if body_type == 0xB5 else 4
        if position + header_length > len(body) - trailer:
            raise MideaBleFrameError("truncated optional-property TLV header")
        tag = body[position] | (body[position + 1] << 8)
        length_index = position + (2 if body_type == 0xB5 else 3)
        length = body[length_index]
        value_start = length_index + 1
        value_end = value_start + length
        if value_end > len(body) - trailer:
            raise MideaBleFrameError("truncated optional-property TLV value")
        if length:
            values[tag] = bytes(body[value_start:value_end])
        position = value_end
    return body_type, values
