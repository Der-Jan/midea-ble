"""Config flow for Midea BLE."""

from __future__ import annotations

from typing import Any

import voluptuous as vol
from homeassistant import config_entries
from homeassistant.components import bluetooth
from homeassistant.config_entries import ConfigFlowResult

from .const import CONF_ADVERTIS_DATA, CONF_SERIAL, DOMAIN
from .protocol.advertisement import (
    MideaAdvertisement,
    is_supported_manufacturer_data,
    parse_manufacturer_data,
)


class MideaBleConfigFlow(config_entries.ConfigFlow, domain=DOMAIN):
    """Handle discovery and user setup."""

    VERSION = 1
    _discovery: bluetooth.BluetoothServiceInfoBleak | None = None
    _parsed: MideaAdvertisement | None = None

    async def async_step_bluetooth(
        self, discovery_info: bluetooth.BluetoothServiceInfoBleak
    ) -> ConfigFlowResult:
        """Handle a connectable Midea advertisement."""
        if not discovery_info.connectable or not is_supported_manufacturer_data(
            discovery_info.manufacturer_data
        ):
            return self.async_abort(reason="not_supported")
        parsed = parse_manufacturer_data(discovery_info.manufacturer_data)
        await self.async_set_unique_id(parsed.unique_id)
        self._abort_if_unique_id_configured()
        self._discovery = discovery_info
        self._parsed = parsed
        self.context["title_placeholders"] = {"name": f"Midea AC {parsed.serial}"}
        return await self.async_step_confirm()

    async def async_step_confirm(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """Confirm a discovered device."""
        if self._parsed is None or self._discovery is None:
            return self.async_abort(reason="not_supported")
        if user_input is not None:
            return self.async_create_entry(
                title=f"Midea AC {self._parsed.serial}",
                data={
                    CONF_SERIAL: self._parsed.serial,
                    CONF_ADVERTIS_DATA: self._parsed.advertis_data.hex(),
                    "address": self._discovery.address,
                },
            )
        return self.async_show_form(
            step_id="confirm",
            data_schema=vol.Schema({}),
            description_placeholders={"name": f"Midea AC {self._parsed.serial}"},
        )

    async def async_step_user(
        self, user_input: dict[str, Any] | None = None
    ) -> ConfigFlowResult:
        """List compatible devices currently visible to connectable adapters."""
        devices: dict[
            str, tuple[bluetooth.BluetoothServiceInfoBleak, MideaAdvertisement]
        ] = {}
        for info in bluetooth.async_discovered_service_info(self.hass, connectable=True):
            if is_supported_manufacturer_data(info.manufacturer_data):
                parsed = parse_manufacturer_data(info.manufacturer_data)
                devices[parsed.unique_id] = (info, parsed)
        if not devices:
            return self.async_abort(reason="no_devices_found")
        if user_input is not None:
            info, _ = devices[user_input["device"]]
            return await self.async_step_bluetooth(info)
        configured = set(self._async_current_ids())
        choices = {
            uid: f"Midea AC {parsed.serial} ({info.address})"
            for uid, (info, parsed) in devices.items()
            if uid not in configured
        }
        if not choices:
            return self.async_abort(reason="already_configured")
        return self.async_show_form(
            step_id="user",
            data_schema=vol.Schema({vol.Required("device"): vol.In(choices)}),
        )
