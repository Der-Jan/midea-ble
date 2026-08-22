"""Golden tests for optional AC appliance-level features."""

from custom_components.midea_ble.protocol.features import (
    ACProperty,
    build_capability_query_frame,
    build_display_toggle_frame,
    build_property_query_frame,
    build_property_set_frame,
    parse_tlv_response,
)

B5_PRIMARY_CAPTURE = bytes.fromhex(
    "aa3dac00000000000803b50a1202010114020101150201001e02010117020102"
    "1a02010110020101250207203c203c203c0024020101480001010101c71a"
)
B1_RATE_CAPTURE = bytes.fromhex(
    "aa14ac00000000000803b1014800000164000000d6"
)


def test_optional_request_golden_packets() -> None:
    assert build_capability_query_frame(1).hex() == "aa0fac00000000000803b5010001e59e"
    assert build_capability_query_frame(2, additional=True).hex() == (
        "aa10ac00000000000803b501010102c1be"
    )
    assert build_property_query_frame((ACProperty.OUTDOOR_SILENT,), 3).hex() == (
        "aa10ac00000000000803b101cd0003ac0b"
    )
    assert build_property_set_frame(ACProperty.OUTDOOR_SILENT, 3, 4).hex() == (
        "aa16ac00000000000802b002cd0001031a00010104533e"
    )
    assert build_display_toggle_frame(5).hex() == (
        "aa20ac00000000000803410200ff020002000000000000000000000000000529b5"
    )


def test_parse_actual_portasplit_b5_and_b1_captures() -> None:
    body_type, capabilities = parse_tlv_response(B5_PRIMARY_CAPTURE)
    assert body_type == 0xB5
    assert capabilities[0x0215] == b"\x00"
    assert capabilities[0x0224] == b"\x01"

    body_type, properties = parse_tlv_response(B1_RATE_CAPTURE)
    assert body_type == 0xB1
    assert properties[ACProperty.RATE_SELECT] == b"\x64"
