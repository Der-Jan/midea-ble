"""Home Assistant coordinator for one Midea BLE air conditioner."""

from __future__ import annotations

import logging
from collections.abc import Awaitable
from datetime import timedelta

from homeassistant.core import HomeAssistant
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .client import ACData, MideaBleClient
from .const import DOMAIN
from .protocol.features import ACOptionalState, ACProperty
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
        self._optional: ACOptionalState | None = None

    async def _async_update_data(self) -> ACData:
        try:
            if self._optional is None:
                self._optional = await self.client.async_probe_optional_features()
            data = await self.client.async_query_data(self._optional)
            self._optional = data.optional
            return data
        except Exception as err:
            raise UpdateFailed(str(err)) from err

    async def _async_update_status(self, update: Awaitable[ACStatus]) -> None:
        status = await update
        energy = self.data.energy if self.data is not None else None
        optional = self.data.optional if self.data is not None else self._optional
        self.async_set_updated_data(
            ACData(status=status, energy=energy, optional=optional)
        )

    async def async_set_optional_property(
        self, prop: ACProperty, value: int
    ) -> None:
        observed = await self.client.async_set_optional_property(prop, value)
        if observed != value:
            raise ValueError(
                f"AC did not confirm property 0x{prop:04x} value {value}"
            )
        current = self.data.optional
        if current is None:
            return
        values = dict(current.values)
        values[prop] = bytes((observed,))
        self._optional = ACOptionalState(values=values, b5_values=current.b5_values)
        self.async_set_updated_data(
            ACData(
                status=self.data.status,
                energy=self.data.energy,
                optional=self._optional,
            )
        )

    async def async_set_display(self, enabled: bool) -> None:
        if self.data.status.screen_display == enabled:
            return
        status = await self.client.async_toggle_display()
        if status.screen_display != enabled:
            raise ValueError("AC did not confirm the requested display state")
        self.async_set_updated_data(
            ACData(status=status, energy=self.data.energy, optional=self.data.optional)
        )

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
