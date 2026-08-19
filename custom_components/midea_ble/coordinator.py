"""Home Assistant coordinator for one Midea BLE air conditioner."""

from __future__ import annotations

import logging
from datetime import timedelta

from homeassistant.core import HomeAssistant
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .client import MideaBleClient
from .const import DOMAIN
from .protocol.status import ACStatus

_LOGGER = logging.getLogger(__name__)


class MideaBleCoordinator(DataUpdateCoordinator[ACStatus]):
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

    async def _async_update_data(self) -> ACStatus:
        try:
            return await self.client.async_query()
        except Exception as err:
            raise UpdateFailed(str(err)) from err

    async def async_set_power(self, power: bool) -> None:
        self.async_set_updated_data(await self.client.async_set_power(power))

    async def async_set_mode(self, mode: int) -> None:
        self.async_set_updated_data(await self.client.async_set_mode(mode))

    async def async_set_temperature(self, temperature: float) -> None:
        self.async_set_updated_data(
            await self.client.async_set_temperature(temperature)
        )

    async def async_set_fan_speed(self, fan_speed: int) -> None:
        self.async_set_updated_data(await self.client.async_set_fan_speed(fan_speed))

    async def async_set_swing(self, up_down: bool, left_right: bool) -> None:
        self.async_set_updated_data(
            await self.client.async_set_swing(up_down, left_right)
        )

    async def async_set_eco(self, enabled: bool) -> None:
        self.async_set_updated_data(await self.client.async_set_eco(enabled))

    async def async_set_strong(self, enabled: bool) -> None:
        self.async_set_updated_data(await self.client.async_set_strong(enabled))
