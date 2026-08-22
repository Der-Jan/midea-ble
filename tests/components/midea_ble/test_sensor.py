"""Power and energy sensor metadata/value tests."""

from typing import cast

from homeassistant.components.sensor import SensorDeviceClass, SensorStateClass
from homeassistant.const import UnitOfEnergy, UnitOfPower

from custom_components.midea_ble.client import ACData
from custom_components.midea_ble.coordinator import MideaBleCoordinator
from custom_components.midea_ble.protocol.energy import ACEnergy
from custom_components.midea_ble.protocol.status import ACStatus
from custom_components.midea_ble.sensor import SENSORS, MideaBleSensor


class FakeCoordinator:
    serial = "12345678AC0001"
    data = ACData(
        status=cast("ACStatus", None),
        energy=ACEnergy(
            realtime_power=363.5,
            current_energy_consumption=32.28,
            total_energy_consumption=91.23,
        ),
    )


def test_sensor_values_and_metadata() -> None:
    coordinator = cast(MideaBleCoordinator, FakeCoordinator())
    sensors = {item.key: MideaBleSensor(coordinator, item) for item in SENSORS}

    power = sensors["realtime_power"]
    assert power.native_value == 363.5
    assert power.device_class == SensorDeviceClass.POWER
    assert power.native_unit_of_measurement == UnitOfPower.WATT
    assert power.state_class == SensorStateClass.MEASUREMENT

    for key, expected in (
        ("current_energy_consumption", 32.28),
        ("total_energy_consumption", 91.23),
    ):
        sensor = sensors[key]
        assert sensor.native_value == expected
        assert sensor.device_class == SensorDeviceClass.ENERGY
        assert sensor.native_unit_of_measurement == UnitOfEnergy.KILO_WATT_HOUR
        assert sensor.state_class == SensorStateClass.TOTAL_INCREASING
