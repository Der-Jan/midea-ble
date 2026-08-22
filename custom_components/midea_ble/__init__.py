"""Midea BLE integration setup."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from homeassistant.config_entries import ConfigEntry
    from homeassistant.core import HomeAssistant

    from .coordinator import MideaBleCoordinator

    type MideaBleConfigEntry = ConfigEntry[MideaBleCoordinator]
else:
    MideaBleConfigEntry = Any

PLATFORMS = ["climate", "sensor"]


async def async_setup_entry(hass: HomeAssistant, entry: MideaBleConfigEntry) -> bool:
    """Set up one AC and verify it with the first coordinator refresh."""
    from .bluetooth import make_ha_dialer
    from .client import MideaBleClient
    from .const import CONF_ADVERTIS_DATA, CONF_SERIAL
    from .coordinator import MideaBleCoordinator

    serial = entry.data[CONF_SERIAL]
    address = entry.data["address"]
    advertis_data = bytes.fromhex(entry.data[CONF_ADVERTIS_DATA])
    dialer = make_ha_dialer(hass, address, f"Midea AC {serial}")
    coordinator = MideaBleCoordinator(
        hass, MideaBleClient(dialer, advertis_data), serial
    )
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = coordinator
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: MideaBleConfigEntry) -> bool:
    """Unload a config entry."""
    return bool(await hass.config_entries.async_unload_platforms(entry, PLATFORMS))
