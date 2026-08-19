"""Home Assistant-aware BLE transport for Midea AC transactions."""

from __future__ import annotations

from bleak_retry_connector import BleakClientWithServiceCache, establish_connection
from homeassistant.components import bluetooth
from homeassistant.core import HomeAssistant

from .client import AsyncTransport, Dialer
from .protocol.exceptions import MideaBleConnectionError
from .transport import BleakTransport


def make_ha_dialer(
    hass: HomeAssistant, address: str, name: str
) -> Dialer:
    """Create a dialer resolving the freshest connectable HA Bluetooth path."""

    async def async_dial() -> AsyncTransport:
        device = bluetooth.async_ble_device_from_address(
            hass, address, connectable=True
        )
        if device is None:
            reason = bluetooth.async_address_reachability_diagnostics(
                hass,
                address,
                bluetooth.BluetoothReachabilityIntent.CONNECTION,
            )
            raise MideaBleConnectionError(f"device is not connectable: {reason}")
        client = await establish_connection(
            BleakClientWithServiceCache,
            device,
            name,
            max_attempts=3,
        )
        return BleakTransport(client)

    return async_dial
