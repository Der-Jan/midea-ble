"""Golden tests ported from internal/ble/scanner.go examples."""

import pytest

from custom_components.midea_ble.protocol.advertisement import (
    is_supported_manufacturer_data,
    parse_manufacturer_data,
    parse_manufacturer_payload,
)
from custom_components.midea_ble.protocol.constants import MIDEA_MANUFACTURER_ID
from custom_components.midea_ble.protocol.exceptions import MideaBleAdvertisementError

PAYLOAD = bytes.fromhex(
    "01" "3132333435363738414330303031" "01030032" "ffeeddccbbaa" "00"
)
SHORT_DEVICE_PAYLOAD = bytes.fromhex("013030303030513136414336393832")


def test_parse_upstream_advertisement_example() -> None:
    parsed = parse_manufacturer_payload(PAYLOAD)
    assert parsed.serial == "12345678AC0001"
    assert parsed.serial_short == "12345678"
    assert parsed.embedded_address == "AA:BB:CC:DD:EE:FF"
    assert parsed.advertis_data.hex() == "ac3132333435363738aabbccddeeff"
    assert parsed.unique_id == "12345678AC0001"


def test_selects_company_id() -> None:
    parsed = parse_manufacturer_data({1: b"noise", MIDEA_MANUFACTURER_ID: PAYLOAD})
    assert parsed.serial == "12345678AC0001"


def test_parses_short_home_assistant_advertisement_with_observed_address() -> None:
    parsed = parse_manufacturer_payload(
        SHORT_DEVICE_PAYLOAD, "60:7A:D8:91:69:83"
    )
    assert parsed.serial == "00000Q16AC6982"
    assert parsed.embedded_address == "60:7A:D8:91:69:83"
    assert parsed.advertis_data.hex() == "ac3030303030513136607ad8916983"


def test_short_advertisement_requires_valid_observed_address() -> None:
    with pytest.raises(MideaBleAdvertisementError):
        parse_manufacturer_payload(SHORT_DEVICE_PAYLOAD)
    with pytest.raises(MideaBleAdvertisementError):
        parse_manufacturer_payload(SHORT_DEVICE_PAYLOAD, "not-an-address")


def test_supported_predicate_accepts_address_assisted_advertisement() -> None:
    data = {MIDEA_MANUFACTURER_ID: SHORT_DEVICE_PAYLOAD}
    assert not is_supported_manufacturer_data(data)
    assert is_supported_manufacturer_data(data, "60:7A:D8:91:69:83")


@pytest.mark.parametrize("payload", [b"", PAYLOAD[:24], bytes([2]) + PAYLOAD[1:]])
def test_rejects_malformed_payload(payload: bytes) -> None:
    with pytest.raises(MideaBleAdvertisementError):
        parse_manufacturer_payload(payload)


def test_supported_predicate_never_raises() -> None:
    assert is_supported_manufacturer_data({MIDEA_MANUFACTURER_ID: PAYLOAD})
    assert not is_supported_manufacturer_data({})
