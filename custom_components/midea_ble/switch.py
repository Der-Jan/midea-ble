"""Capability-gated switches for a Midea BLE air conditioner."""

from __future__ import annotations

from dataclasses import dataclass

from homeassistant.components.switch import SwitchDeviceClass, SwitchEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import MideaBleConfigEntry
from .const import DOMAIN
from .coordinator import MideaBleCoordinator
from .protocol.features import ACProperty, B5Capability


@dataclass(frozen=True, slots=True)
class SwitchDescription:
    key: str
    name: str
    prop: ACProperty | None
    enabled_value: int = 1


OUTDOOR_SILENT = SwitchDescription(
    key="outdoor_silent", name="Outdoor silent", prop=ACProperty.OUTDOOR_SILENT, enabled_value=3
)
SELF_CLEAN = SwitchDescription(key="self_clean", name="Self-clean", prop=ACProperty.SELF_CLEAN)
DISPLAY = SwitchDescription(key="screen_display", name="Screen display", prop=None)


async def async_setup_entry(
    hass: HomeAssistant,
    entry: MideaBleConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    coordinator = entry.runtime_data
    optional = coordinator.data.optional
    if optional is None:
        return
    descriptions: list[SwitchDescription] = []
    if (
        optional.b5_values.get(ACProperty.OUTDOOR_SILENT) == b"\x03"
        and ACProperty.OUTDOOR_SILENT in optional.values
    ):
        descriptions.append(OUTDOOR_SILENT)
    if (
        optional.b5_values.get(B5Capability.SELF_CLEAN) == b"\x01"
        and ACProperty.SELF_CLEAN in optional.values
    ):
        descriptions.append(SELF_CLEAN)
    display = optional.b5_values.get(B5Capability.DISPLAY)
    if display and display[0] in (1, 2, 100):
        descriptions.append(DISPLAY)
    async_add_entities(MideaBleFeatureSwitch(coordinator, item) for item in descriptions)


class MideaBleFeatureSwitch(CoordinatorEntity[MideaBleCoordinator], SwitchEntity):
    _attr_has_entity_name = True
    _attr_device_class = SwitchDeviceClass.SWITCH

    def __init__(self, coordinator: MideaBleCoordinator, description: SwitchDescription) -> None:
        super().__init__(coordinator)
        self._description = description
        self._attr_name = description.name
        self._attr_unique_id = f"{coordinator.serial}_{description.key}"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, coordinator.serial)},
            name=f"Midea AC {coordinator.serial}",
            manufacturer="Midea",
        )

    @property
    def is_on(self) -> bool:
        if self._description.prop is None:
            return self.coordinator.data.status.screen_display
        optional = self.coordinator.data.optional
        return bool(
            optional
            and optional.value(self._description.prop) == self._description.enabled_value
        )

    @property
    def available(self) -> bool:
        if self._description.prop == ACProperty.SELF_CLEAN:
            return super().available
        return super().available and self.coordinator.data.status.power

    async def async_turn_on(self, **kwargs: object) -> None:
        if self._description.prop is None:
            await self.coordinator.async_set_display(True)
        else:
            await self.coordinator.async_set_optional_property(
                self._description.prop, self._description.enabled_value
            )

    async def async_turn_off(self, **kwargs: object) -> None:
        if self._description.prop is None:
            await self.coordinator.async_set_display(False)
        else:
            await self.coordinator.async_set_optional_property(self._description.prop, 0)
