"""Climate entity for a Midea BLE air conditioner."""

from __future__ import annotations

from typing import Any

from homeassistant.components.climate import ClimateEntity
from homeassistant.components.climate.const import ClimateEntityFeature, HVACMode
from homeassistant.const import ATTR_TEMPERATURE, UnitOfTemperature
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import MideaBleConfigEntry
from .const import DOMAIN
from .coordinator import MideaBleCoordinator
from .protocol.commands import (
    MODE_AUTO,
    MODE_COOL,
    MODE_DRY,
    MODE_FAN,
    MODE_HEAT,
    WIND_AUTO,
    WIND_FULL,
    WIND_HIGH,
    WIND_LOW,
    WIND_MID,
)

MODE_TO_HVAC = {
    MODE_AUTO: HVACMode.AUTO,
    MODE_COOL: HVACMode.COOL,
    MODE_DRY: HVACMode.DRY,
    MODE_HEAT: HVACMode.HEAT,
    MODE_FAN: HVACMode.FAN_ONLY,
}
HVAC_TO_MODE = {value: key for key, value in MODE_TO_HVAC.items()}
FAN_TO_NAME = {
    WIND_LOW: "low",
    WIND_MID: "medium",
    WIND_HIGH: "high",
    WIND_FULL: "full",
    WIND_AUTO: "auto",
}
NAME_TO_FAN = {value: key for key, value in FAN_TO_NAME.items()}
SWING_OFF = "off"
SWING_VERTICAL = "vertical"
SWING_HORIZONTAL = "horizontal"
SWING_BOTH = "both"


async def async_setup_entry(
    hass: HomeAssistant,
    entry: MideaBleConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Create the climate entity for a config entry."""
    async_add_entities([MideaBleClimate(entry.runtime_data)])


class MideaBleClimate(CoordinatorEntity[MideaBleCoordinator], ClimateEntity):
    """Represent one Midea BLE AC."""

    _attr_has_entity_name = True
    _attr_name = None
    _attr_temperature_unit = UnitOfTemperature.CELSIUS
    _attr_min_temp = 16
    _attr_max_temp = 30
    _attr_target_temperature_step = 0.5
    _attr_supported_features = (
        ClimateEntityFeature.TARGET_TEMPERATURE
        | ClimateEntityFeature.FAN_MODE
        | ClimateEntityFeature.SWING_MODE
        | ClimateEntityFeature.TURN_ON
        | ClimateEntityFeature.TURN_OFF
    )

    def __init__(self, coordinator: MideaBleCoordinator) -> None:
        super().__init__(coordinator)
        self._attr_hvac_modes = [HVACMode.OFF, *MODE_TO_HVAC.values()]
        self._attr_fan_modes = list(NAME_TO_FAN)
        self._attr_swing_modes = [
            SWING_OFF,
            SWING_VERTICAL,
            SWING_HORIZONTAL,
            SWING_BOTH,
        ]
        self._attr_unique_id = coordinator.serial
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, coordinator.serial)},
            name=f"Midea AC {coordinator.serial}",
            manufacturer="Midea",
        )

    @property
    def hvac_mode(self) -> HVACMode:
        if not self.coordinator.data.power:
            return HVACMode.OFF
        return MODE_TO_HVAC.get(self.coordinator.data.mode, HVACMode.AUTO)

    @property
    def target_temperature(self) -> float:
        return self.coordinator.data.target_temperature

    @property
    def current_temperature(self) -> float:
        return self.coordinator.data.current_temperature

    @property
    def fan_mode(self) -> str:
        return FAN_TO_NAME.get(self.coordinator.data.fan_speed, "auto")

    @property
    def swing_mode(self) -> str:
        status = self.coordinator.data
        if status.swing_up_down and status.swing_left_right:
            return SWING_BOTH
        if status.swing_up_down:
            return SWING_VERTICAL
        if status.swing_left_right:
            return SWING_HORIZONTAL
        return SWING_OFF

    async def async_set_hvac_mode(self, hvac_mode: HVACMode) -> None:
        if hvac_mode == HVACMode.OFF:
            await self.coordinator.async_set_power(False)
            return
        await self.coordinator.async_set_mode(HVAC_TO_MODE[hvac_mode])

    async def async_set_temperature(self, **kwargs: Any) -> None:
        if (temperature := kwargs.get(ATTR_TEMPERATURE)) is not None:
            await self.coordinator.async_set_temperature(float(temperature))

    async def async_set_fan_mode(self, fan_mode: str) -> None:
        await self.coordinator.async_set_fan_speed(NAME_TO_FAN[fan_mode])

    async def async_set_swing_mode(self, swing_mode: str) -> None:
        await self.coordinator.async_set_swing(
            swing_mode in (SWING_VERTICAL, SWING_BOTH),
            swing_mode in (SWING_HORIZONTAL, SWING_BOTH),
        )

    async def async_turn_on(self) -> None:
        await self.coordinator.async_set_power(True)

    async def async_turn_off(self) -> None:
        await self.coordinator.async_set_power(False)
