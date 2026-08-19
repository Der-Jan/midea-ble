"""Climate entity state and command mapping tests."""

import asyncio
from typing import cast

from homeassistant.components.climate.const import HVACMode

from custom_components.midea_ble.climate import (
    SWING_BOTH,
    MideaBleClimate,
)
from custom_components.midea_ble.coordinator import MideaBleCoordinator
from custom_components.midea_ble.protocol.status import ACStatus


class FakeCoordinator:
    def __init__(self) -> None:
        self.serial = "12345678AC0001"
        self.data = ACStatus(
            power=True,
            mode=2,
            target_temperature=24.5,
            fan_speed=102,
            swing_up_down=True,
            swing_left_right=True,
            eco=False,
            strong=False,
            electric_heat=False,
            target_temperature_2=24.5,
            current_temperature=23.7,
            outdoor_temperature=18.2,
        )
        self.power_commands: list[bool] = []

    async def async_set_power(self, power: bool) -> None:
        self.power_commands.append(power)


def test_climate_maps_status_and_power_command() -> None:
    async def run() -> None:
        coordinator = FakeCoordinator()
        entity = MideaBleClimate(cast(MideaBleCoordinator, coordinator))
        assert entity.hvac_mode == HVACMode.COOL
        assert entity.target_temperature == 24.5
        assert entity.current_temperature == 23.7
        assert entity.fan_mode == "auto"
        assert entity.swing_mode == SWING_BOTH
        await entity.async_turn_off()
        assert coordinator.power_commands == [False]

    asyncio.run(run())
