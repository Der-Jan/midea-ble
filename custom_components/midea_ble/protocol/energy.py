"""Decode Midea AC C1/group-4 power and energy responses."""

from __future__ import annotations

from dataclasses import dataclass
from enum import IntEnum

from .exceptions import MideaBleFrameError


class PowerAnalysisMethod(IntEnum):
    """Known Midea encodings for C1 power data."""

    BCD = 1
    BINARY = 2
    RADIX_100 = 3
    PORTASPLIT = 12
    BCD_ENERGY_BINARY_POWER = 101


@dataclass(frozen=True, slots=True)
class ACEnergy:
    """Power and energy values reported by an AC."""

    realtime_power: float
    current_energy_consumption: float
    total_energy_consumption: float


def _validate_frame(frame: bytes) -> bytes:
    if len(frame) < 30:
        raise MideaBleFrameError("appliance energy frame is too short")
    if frame[0] != 0xAA:
        raise MideaBleFrameError("appliance frame sync mismatch")
    expected = frame[1] + 1
    if len(frame) != expected:
        raise MideaBleFrameError(
            f"appliance length mismatch: got {len(frame)}, expected {expected}"
        )
    if sum(frame[1:]) & 0xFF:
        raise MideaBleFrameError("appliance checksum mismatch")
    if frame[9] != 0x03 or frame[10] != 0xC1:
        raise MideaBleFrameError("expected an AC C1 query response")
    body = frame[10:]
    if body[3] != 0x44:
        raise MideaBleFrameError(f"unexpected C1 group response: 0x{body[3]:02x}")
    return body


def _decode_value(method: PowerAnalysisMethod, data: bytes) -> int:
    base_method = int(method) % 10
    value = 0
    for byte in data:
        if base_method == PowerAnalysisMethod.BCD:
            value = value * 100 + (byte >> 4) * 10 + (byte & 0x0F)
        elif base_method == PowerAnalysisMethod.BINARY:
            value = (value << 8) + byte
        elif base_method == PowerAnalysisMethod.RADIX_100:
            value = value * 100 + byte
        else:  # pragma: no cover - every enum member maps to 1, 2, or 3
            raise ValueError(f"unsupported power analysis method: {method}")
    return value


def _decode_energy(method: PowerAnalysisMethod, data: bytes) -> float:
    encoding = (
        PowerAnalysisMethod.BCD if method == PowerAnalysisMethod.BCD_ENERGY_BINARY_POWER else method
    )
    divisor = 10 if method == PowerAnalysisMethod.BINARY else 100
    return _decode_value(encoding, data) / divisor


def _decode_power(method: PowerAnalysisMethod, data: bytes) -> float:
    encoding = (
        PowerAnalysisMethod.BINARY
        if method == PowerAnalysisMethod.BCD_ENERGY_BINARY_POWER
        else method
    )
    return _decode_value(encoding, data) / 10


def parse_energy_frame(
    frame: bytes,
    method: PowerAnalysisMethod = PowerAnalysisMethod.PORTASPLIT,
) -> ACEnergy:
    """Validate and decode a complete C1/group-4 appliance response.

    Group 0x44 stores total energy in body bytes 4..7, current energy in
    bytes 12..15, and realtime power in bytes 16..18. PortaSplit method 12
    treats all fields as big-endian binary, with 0.01 kWh and 0.1 W units.
    """
    body = _validate_frame(frame)
    return ACEnergy(
        total_energy_consumption=_decode_energy(method, body[4:8]),
        current_energy_consumption=_decode_energy(method, body[12:16]),
        realtime_power=_decode_power(method, body[16:19]),
    )
