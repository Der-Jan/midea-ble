"""Bleak transport shared by Home Assistant and the development probe."""

from __future__ import annotations

from contextlib import suppress

from bleak import BleakClient

from .client import AsyncTransport, NotifyCallback
from .protocol.constants import MIDEA_NOTIFY_CHAR_UUID, MIDEA_WRITE_CHAR_UUID


class BleakTransport(AsyncTransport):
    """Adapt one connected Bleak client to protocol byte operations."""

    def __init__(self, client: BleakClient) -> None:
        self._client = client
        self._notifying = False

    async def start_notify(self, callback: NotifyCallback) -> None:
        """Subscribe to FFA2 notifications/indications."""

        def on_notify(_sender: object, data: bytearray) -> None:
            callback(bytes(data))

        await self._client.start_notify(MIDEA_NOTIFY_CHAR_UUID, on_notify)
        self._notifying = True

    async def write(self, data: bytes) -> None:
        """Write to FFA1 with response, as required by the device."""
        await self._client.write_gatt_char(MIDEA_WRITE_CHAR_UUID, data, response=True)

    async def close(self) -> None:
        """Unsubscribe and disconnect safely."""
        if self._notifying and self._client.is_connected:
            with suppress(Exception):  # Device may already have disconnected.
                await self._client.stop_notify(MIDEA_NOTIFY_CHAR_UUID)
        if self._client.is_connected:
            await self._client.disconnect()
