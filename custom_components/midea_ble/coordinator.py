"""Home Assistant coordinator for one Midea BLE air conditioner."""

from __future__ import annotations

import logging
from collections.abc import Awaitable
from datetime import timedelta

from homeassistant.core import HomeAssistant
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .client import ACData, MideaBleClient
from .const import DOMAIN
from .protocol.status import ACStatus

_LOGGER = logging.getLogger(__name__)


class MideaBleCoordinator(DataUpdateCoordinator[ACData]):
    """Poll and mutate one AC through serialized one-shot BLE transactions."""

    def __init__(self, hass: HomeAssistant, client: MideaBleClient, serial: str) -> None:
        super().__init__(
            hass,
            logger=_LOGGER,
            name=f"{DOMAIN}_{serial}",
            update_interval=timedelta(seconds=60),
        )
        self.client = client
        self.serial = serial

    async def _async_update_data(self) -> ACData:
        try:
            return await self.client.async_query_data()
        except Exception as err:
            raise UpdateFailed(str(err)) from err

    async def _async_update_status(self, update: Awaitable[ACStatus]) -> None:
        status = await update
        energy = self.data.energy if self.data is not None else None
        self.async_set_updated_data(ACData(status=status, energy=energy))

    async def async_set_power(self, power: bool) -> None:
        await self._async_update_status(self.client.async_set_power(power))

    async def async_set_mode(self, mode: int) -> None:
        await self._async_update_status(self.client.async_set_mode(mode))

    async def async_set_temperature(self, temperature: float) -> None:
        await self._async_update_status(
            self.client.async_set_temperature(temperature)
        )

    async def async_set_fan_speed(self, fan_speed: int) -> None:
        await self._async_update_status(self.client.async_set_fan_speed(fan_speed))

    async def async_set_swing(self, up_down: bool, left_right: bool) -> None:
        await self._async_update_status(
            self.client.async_set_swing(up_down, left_right)
        )

    async def async_set_eco(self, enabled: bool) -> None:
        await self._async_update_status(self.client.async_set_eco(enabled))

    async def async_set_strong(self, enabled: bool) -> None:
        await self._async_update_status(self.client.async_set_strong(enabled))
