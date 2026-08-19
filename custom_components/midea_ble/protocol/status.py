"""Parse Midea AC opcode-0xC0 appliance status frames.

Ported from ``internal/ac/appliance.go`` in midea-ble-go.
"""

import math
from dataclasses import dataclass

from .commands import ACState
from .exceptions import MideaBleFrameError


@dataclass(frozen=True, slots=True)
class ACStatus:
    """Control-relevant state reported by an AC."""

    power: bool
    mode: int
    target_temperature: float
    fan_speed: int
    swing_up_down: bool
    swing_left_right: bool
    eco: bool
    strong: bool
    electric_heat: bool
    target_temperature_2: float
    current_temperature: float
    outdoor_temperature: float

    def as_control_state(self) -> ACState:
        """Create a mutable full-control state while preserving reported fields."""
        return ACState(
            power=self.power,
            mode=self.mode,
            target_temperature=self.target_temperature,
            fan_speed=self.fan_speed,
            swing_up_down=self.swing_up_down,
            swing_left_right=self.swing_left_right,
            eco=self.eco,
            strong=self.strong,
            electric_heat=self.electric_heat,
            target_temperature_2=self.target_temperature_2,
        )


def parse_status_frame(frame: bytes) -> ACStatus:
    """Validate and decode one complete appliance status frame."""
    if len(frame) < 27:
        raise MideaBleFrameError("appliance status frame is too short")
    if frame[0] != 0xAA:
        raise MideaBleFrameError("appliance frame sync mismatch")
    expected = frame[1] + 1
    if len(frame) != expected:
        raise MideaBleFrameError(
            f"appliance length mismatch: got {len(frame)}, expected {expected}"
        )
    if sum(frame[1:]) & 0xFF:
        raise MideaBleFrameError("appliance checksum mismatch")
    if frame[10] != 0xC0:
        raise MideaBleFrameError(f"unexpected appliance opcode: 0x{frame[10]:02x}")
    status = frame[10:]
    swing = status[7]
    swing_left_right = (swing & 0xF0) == 0x30 and bool(swing & 0x03)
    swing_up_down = (swing & 0xF0) == 0x30 and bool(swing & 0x0C)
    target_temperature = float(16 + (status[2] & 0x0F))
    if status[2] & 0x10:
        target_temperature += 0.5
    target_temperature_2 = float(12 + (status[13] & 0x1F))
    if status[2] & 0x10:
        target_temperature_2 += 0.5
    current_temperature = float(math.trunc((status[11] - 50) / 2))
    indoor_decimal = 0.1 * (status[15] & 0x0F)
    current_temperature += (
        -indoor_decimal if current_temperature < 0 else indoor_decimal
    )
    outdoor_temperature = (status[12] - 50) / 2
    outdoor_temperature += 0.1 * ((status[15] >> 4) & 0x0F)
    return ACStatus(
        power=bool(status[1] & 1),
        mode=(status[2] & 0xE0) >> 5,
        target_temperature=target_temperature,
        fan_speed=status[3],
        swing_up_down=swing_up_down,
        swing_left_right=swing_left_right,
        strong=bool(status[8] & 0x20),
        electric_heat=bool(status[9] & 0x08),
        eco=bool(status[9] & 0x10),
        target_temperature_2=target_temperature_2,
        current_temperature=round(current_temperature, 1),
        outdoor_temperature=round(outdoor_temperature, 1),
    )
