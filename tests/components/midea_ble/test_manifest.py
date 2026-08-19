"""Repository and integration metadata tests."""

import json
from pathlib import Path

ROOT = Path(__file__).parents[3]


def test_manifest_has_connectable_protocol_matcher() -> None:
    manifest = json.loads(
        (ROOT / "custom_components/midea_ble/manifest.json").read_text()
    )
    assert manifest["domain"] == "midea_ble"
    assert manifest["config_flow"] is True
    assert manifest["integration_type"] == "device"
    assert manifest["iot_class"] == "local_push"
    assert manifest["dependencies"] == ["bluetooth", "bluetooth_adapters"]
    assert manifest["requirements"] == ["bleak-retry-connector==4.6.3"]
    assert manifest["bluetooth"] == [
        {
            "connectable": True,
            "manufacturer_id": 0x06A8,
            "manufacturer_data_start": [1],
        }
    ]


def test_hacs_layout_contains_exactly_one_integration() -> None:
    hacs = json.loads((ROOT / "hacs.json").read_text())
    integrations = [path for path in (ROOT / "custom_components").iterdir() if path.is_dir()]
    assert hacs["name"] == "Midea BLE"
    assert hacs["homeassistant"] == "2026.8.0"
    assert [path.name for path in integrations] == ["midea_ble"]
