"""Power and energy sensors for a Midea BLE air conditioner."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

from homeassistant.components.sensor import (
    SensorDeviceClass,
    SensorEntity,
    SensorStateClass,
)
from homeassistant.const import UnitOfEnergy, UnitOfPower
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity_platform import AddEntitiesCallback
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from . import MideaBleConfigEntry
from .const import DOMAIN
from .coordinator import MideaBleCoordinator
from .protocol.energy import ACEnergy


@dataclass(frozen=True, kw_only=True)
class MideaBleSensorDescription:
    """Describe one AC power or energy value."""

    key: str
    name: str
    device_class: SensorDeviceClass
    native_unit_of_measurement: str
    state_class: SensorStateClass
    value_fn: Callable[[ACEnergy], float]


SENSORS = (
    MideaBleSensorDescription(
        key="realtime_power",
        name="Realtime power",
        device_class=SensorDeviceClass.POWER,
        native_unit_of_measurement=UnitOfPower.WATT,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda energy: energy.realtime_power,
    ),
    MideaBleSensorDescription(
        key="current_energy_consumption",
        name="Current energy consumption",
        device_class=SensorDeviceClass.ENERGY,
        native_unit_of_measurement=UnitOfEnergy.KILO_WATT_HOUR,
        state_class=SensorStateClass.TOTAL_INCREASING,
        value_fn=lambda energy: energy.current_energy_consumption,
    ),
    MideaBleSensorDescription(
        key="total_energy_consumption",
        name="Total energy consumption",
        device_class=SensorDeviceClass.ENERGY,
        native_unit_of_measurement=UnitOfEnergy.KILO_WATT_HOUR,
        state_class=SensorStateClass.TOTAL_INCREASING,
        value_fn=lambda energy: energy.total_energy_consumption,
    ),
)


async def async_setup_entry(
    hass: HomeAssistant,
    entry: MideaBleConfigEntry,
    async_add_entities: AddEntitiesCallback,
) -> None:
    """Create energy sensors when the AC answered the initial energy query."""
    coordinator = entry.runtime_data
    if coordinator.data.energy is not None:
        async_add_entities(MideaBleSensor(coordinator, description) for description in SENSORS)


class MideaBleSensor(CoordinatorEntity[MideaBleCoordinator], SensorEntity):
    """Represent one power-related value from a Midea BLE AC."""

    _attr_has_entity_name = True

    def __init__(
        self,
        coordinator: MideaBleCoordinator,
        description: MideaBleSensorDescription,
    ) -> None:
        super().__init__(coordinator)
        self._description = description
        self._attr_name = description.name
        self._attr_unique_id = f"{coordinator.serial}_{description.key}"
        self._attr_device_class = description.device_class
        self._attr_native_unit_of_measurement = description.native_unit_of_measurement
        self._attr_state_class = description.state_class
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, coordinator.serial)},
            name=f"Midea AC {coordinator.serial}",
            manufacturer="Midea",
        )

    @property
    def native_value(self) -> float | None:
        energy = self.coordinator.data.energy
        return None if energy is None else self._description.value_fn(energy)

    @property
    def available(self) -> bool:
        return super().available and self.coordinator.data.energy is not None
