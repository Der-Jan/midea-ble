"""Golden appliance-frame tests ported from internal/ac/appliance_test.go."""

from custom_components.midea_ble.protocol.commands import (
    ACState,
    build_control_frame,
    build_energy_query_frame,
    build_query_frame,
)
from custom_components.midea_ble.protocol.status import parse_status_frame


def test_query_frame_matches_go_vector() -> None:
    assert build_query_frame(1).hex() == "aa17ac00000000000003412100ff03ff000200000001a92b"


def test_energy_query_matches_midea_group_four_vector() -> None:
    assert build_energy_query_frame().hex() == "aa11ac00000000000003412101440001098f"


def test_control_frame_matches_go_vector() -> None:
    state = ACState(power=True)
    assert build_control_frame(state, 1).hex() == (
        "aa24ac0000000000020240434a667f7fff300000009999999999000a0e800000"
        "0000017db9"
    )


def test_parse_status_go_vector() -> None:
    raw = bytes.fromhex(
        "aa2aac00000000000803c0014b667f7f0000000000676c0f0037000000000000"
        "000000000000b7ff424c52"
    )
    status = parse_status_frame(raw)
    assert status.power
    assert status.mode == 2
    assert status.target_temperature == 27
    assert status.fan_speed == 102
