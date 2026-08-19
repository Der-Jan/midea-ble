"""Parse Midea manufacturer advertisements without Home Assistant imports.

Ported from ``internal/ble/scanner.go`` (``buildAdvertisData`` and
``mfData06A8``) in midea-ble-go.
"""

import re
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


def _address_bytes(address: str) -> bytes:
    """Parse a Bluetooth MAC address in Home Assistant's canonical form."""
    if re.fullmatch(r"(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}", address) is None:
        raise MideaBleAdvertisementError(
            f"invalid Bluetooth address for short advertisement: {address}"
        )
    return bytes.fromhex(address.replace(":", ""))


def parse_manufacturer_payload(
    payload: bytes, fallback_address: str | None = None
) -> MideaAdvertisement:
    """Parse a complete or address-assisted 0x06A8 payload.

    Upstream requires 25 bytes and marker 0x01. Bytes 1:15 are the advertised
    serial, 1:9 its root-key short serial, and 19:25 the MAC in reverse order.
    Some scanners only expose the 15-byte marker-and-serial advertisement. In
    that case Home Assistant's observed BLE address supplies the missing MAC.
    """
    if len(payload) < 15:
        raise MideaBleAdvertisementError(
            f"manufacturer payload too short: {len(payload)} (expected >= 15)"
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

    if len(payload) >= 25:
        address_bytes = payload[19:25][::-1]
    elif fallback_address is not None:
        address_bytes = _address_bytes(fallback_address)
    else:
        raise MideaBleAdvertisementError(
            "short manufacturer payload requires the observed Bluetooth address"
        )
    address = ":".join(f"{part:02X}" for part in address_bytes)
    advertis_data = b"\xac" + payload[1:9] + address_bytes
    return MideaAdvertisement(
        serial=serial,
        serial_short=payload[1:9].decode("ascii"),
        embedded_address=address,
        advertis_data=advertis_data,
        manufacturer_data=bytes(payload),
    )


def parse_manufacturer_data(
    data: Mapping[int, bytes], fallback_address: str | None = None
) -> MideaAdvertisement:
    """Select and parse the Midea company entry from manufacturer data."""
    try:
        payload = data[MIDEA_MANUFACTURER_ID]
    except KeyError as err:
        raise MideaBleAdvertisementError("Midea manufacturer data is absent") from err
    return parse_manufacturer_payload(payload, fallback_address)


def is_supported_manufacturer_data(
    data: Mapping[int, bytes], fallback_address: str | None = None
) -> bool:
    """Return whether manufacturer data identifies a supported device."""
    try:
        parse_manufacturer_data(data, fallback_address)
    except MideaBleAdvertisementError:
        return False
    return True
