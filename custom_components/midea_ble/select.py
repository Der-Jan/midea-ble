"""Capability-gated power-rate selector for Midea BLE."""

from __future__ import annotations

from homeassistant.components.select import SelectEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import MideaBleConfigEntry
from .const import DOMAIN
from .coordinator import MideaBleCoordinator
from .protocol.features import ACProperty, B5Capability

RATE_OPTIONS = ["1", "20", "40", "60", "80", "100"]


async def async_setup_entry(
    hass: HomeAssistant,
    entry: MideaBleConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    coordinator = entry.runtime_data
    optional = coordinator.data.optional
    if optional is None:
        return
    electricity = optional.b5_values.get(B5Capability.ELECTRICITY)
    if electricity and electricity[0] > 0 and ACProperty.RATE_SELECT in optional.values:
        async_add_entities([MideaBleRateSelect(coordinator)])


class MideaBleRateSelect(CoordinatorEntity[MideaBleCoordinator], SelectEntity):
    _attr_has_entity_name = True
    _attr_name = "Power rate limit"
    _attr_options = RATE_OPTIONS

    def __init__(self, coordinator: MideaBleCoordinator) -> None:
        super().__init__(coordinator)
        self._attr_unique_id = f"{coordinator.serial}_power_rate_limit"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, coordinator.serial)},
            name=f"Midea AC {coordinator.serial}",
            manufacturer="Midea",
        )

    @property
    def current_option(self) -> str | None:
        optional = self.coordinator.data.optional
        value = optional.value(ACProperty.RATE_SELECT) if optional else None
        return str(value) if value is not None else None

    @property
    def available(self) -> bool:
        return super().available and self.coordinator.data.status.power

    async def async_select_option(self, option: str) -> None:
        await self.coordinator.async_set_optional_property(
            ACProperty.RATE_SELECT, int(option)
        )
