"""Parse Midea manufacturer advertisements without Home Assistant imports.

Ported from ``internal/ble/scanner.go`` (``buildAdvertisData`` and
``mfData06A8``) in midea-ble-go.
"""

from collections.abc import Mapping
from dataclasses import dataclass

from .constants import MIDEA_ADVERTISEMENT_MARKER, MIDEA_MANUFACTURER_ID
from .exceptions import MideaBleAdvertisementError


@dataclass(frozen=True, slots=True)
class MideaAdvertisement:
    """Validated fields used for discovery and root-key derivation."""

    serial: str
    serial_short: str
    embedded_address: str
    advertis_data: bytes
    manufacturer_data: bytes

    @property
    def unique_id(self) -> str:
        """Return the stable protocol identifier preferred by config flow."""
        return self.serial


def parse_manufacturer_payload(payload: bytes) -> MideaAdvertisement:
    """Parse a complete 0x06A8 payload.

    Upstream requires 25 bytes and marker 0x01. Bytes 1:15 are the advertised
    serial, 1:9 its root-key short serial, and 19:25 the MAC in reverse order.
    """
    if len(payload) < 25:
        raise MideaBleAdvertisementError(
            f"manufacturer payload too short: {len(payload)} (expected >= 25)"
        )
    if payload[0] != MIDEA_ADVERTISEMENT_MARKER:
        raise MideaBleAdvertisementError(
            f"unexpected advertisement marker: 0x{payload[0]:02x}"
        )
    serial_bytes = payload[1:15]
    try:
        serial = serial_bytes.decode("ascii")
    except UnicodeDecodeError as err:
        raise MideaBleAdvertisementError("serial is not ASCII") from err
    if not serial.isprintable():
        raise MideaBleAdvertisementError("serial contains non-printable characters")

    address_bytes = payload[19:25][::-1]
    address = ":".join(f"{part:02X}" for part in address_bytes)
    advertis_data = b"\xac" + payload[1:9] + address_bytes
    return MideaAdvertisement(
        serial=serial,
        serial_short=payload[1:9].decode("ascii"),
        embedded_address=address,
        advertis_data=advertis_data,
        manufacturer_data=bytes(payload),
    )


def parse_manufacturer_data(data: Mapping[int, bytes]) -> MideaAdvertisement:
    """Select and parse the Midea company entry from manufacturer data."""
    try:
        payload = data[MIDEA_MANUFACTURER_ID]
    except KeyError as err:
        raise MideaBleAdvertisementError("Midea manufacturer data is absent") from err
    return parse_manufacturer_payload(payload)


def is_supported_manufacturer_data(data: Mapping[int, bytes]) -> bool:
    """Return whether manufacturer data contains a complete supported payload."""
    try:
        parse_manufacturer_data(data)
    except MideaBleAdvertisementError:
        return False
    return True
