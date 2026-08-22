"""Golden tests for Midea AC C1/group-4 energy reports."""

import pytest

from custom_components.midea_ble.protocol.energy import (
    PowerAnalysisMethod,
    parse_energy_frame,
)
from custom_components.midea_ble.protocol.exceptions import MideaBleFrameError

# Captured from a Midea PortaSplit 00000Q1D subtype 524. The original LAN
# transport delivered this complete inner appliance packet; only that AA frame
# is reused by BLE, not the LAN transport envelope.
PORTASPLIT_CAPTURE = bytes.fromhex(
    "aa22ac00000000000803c121014400000c9c0000000000000c9c000e3300000001f975"
)

# Captured through this integration's encrypted BLE session from serial
# 00000Q16AC6982 while the appliance was off.
BLE_PORTASPLIT_CAPTURE = bytes.fromhex(
    "aa22ac00000000000803c121014400000017000000000000000000000f000000015d7c"
)


def _captured_frame(body: str) -> bytes:
    frame = bytearray.fromhex("aa22ac00000000000803" + body + "00")
    frame[-1] = -sum(frame[1:-1]) & 0xFF
    return bytes(frame)


def test_parse_portasplit_method_12_capture() -> None:
    energy = parse_energy_frame(PORTASPLIT_CAPTURE, PowerAnalysisMethod.PORTASPLIT)
    assert energy.total_energy_consumption == 32.28
    assert energy.current_energy_consumption == 32.28
    assert energy.realtime_power == 363.5


def test_parse_portasplit_capture_received_over_ble() -> None:
    energy = parse_energy_frame(BLE_PORTASPLIT_CAPTURE)
    assert energy.total_energy_consumption == 0.23
    assert energy.current_energy_consumption == 0.0
    assert energy.realtime_power == 1.5


def test_parse_known_bcd_energy_binary_power_capture() -> None:
    # Capture published with midea-local's method-101 regression vectors.
    frame = _captured_frame("c12101440000005800000000000000160016b700000001a9")
    energy = parse_energy_frame(frame, PowerAnalysisMethod.BCD_ENERGY_BINARY_POWER)
    assert energy.total_energy_consumption == 0.58
    assert energy.current_energy_consumption == 0.16
    assert energy.realtime_power == 581.5


def test_rejects_non_energy_c1_group() -> None:
    frame = bytearray(PORTASPLIT_CAPTURE)
    frame[13] = 0x45
    frame[-1] = -sum(frame[1:-1]) & 0xFF
    with pytest.raises(MideaBleFrameError, match="unexpected C1 group"):
        parse_energy_frame(bytes(frame))
