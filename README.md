# Midea BLE for Home Assistant

[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

An experimental HACS-compatible Home Assistant custom integration for direct,
local control of compatible Midea and Hualing Bluetooth air conditioners. It
does not require the Midea app or a cloud connection.

The integration provides Bluetooth discovery, native Python authentication and
protocol handling, a climate entity for operating mode, target temperature,
fan speed, and swing control, and PortaSplit power/energy sensors.

Capability-gated PortaSplit controls are also available for Outdoor Silent,
power-rate limiting, screen display, and self-clean. See
[optional appliance commands](docs/optional-features.md) for the tested support
matrix and intentionally unexposed commands.

Compatibility is limited to devices that use the BLE protocol implemented by
this repository. Support for one Midea AC has been verified; this does not imply
support for every Midea product.

## Requirements

- Home Assistant 2026.8.0 or newer.
- A local Bluetooth adapter or connectable ESPHome Bluetooth proxy within range
  of the air conditioner.
- A compatible AC advertising Midea manufacturer data under company ID
  `0x06A8`.

Bluetooth proxies must support active connections. Passive-only proxies can see
advertisements but cannot control the device.

## Installation with HACS

1. Open HACS in Home Assistant.
2. Open the three-dot menu and select **Custom repositories**.
3. Add `https://github.com/Der-Jan/midea-ble` as an **Integration**.
4. Install **Midea BLE**.
5. Restart Home Assistant.
6. Open **Settings → Devices & services → Add integration → Midea BLE**.
7. Select the discovered air conditioner and confirm setup.

Home Assistant may also present the device automatically as a discovered
integration.

## Supported controls

- Power on and off
- Operating mode
- Target temperature
- Fan speed
- Swing mode
- Status updates

## Bluetooth discovery

The integration accepts both the complete Midea advertisement and the shorter
marker-and-serial advertisement exposed by some Home Assistant scanners. For the
short form, it reconstructs the handshake input from Home Assistant's observed
Bluetooth address.

If no device is found:

- Confirm that the AC appears under **Settings → Bluetooth → Advertisements**.
- Power-cycle the AC and close any phone app currently connected to it.
- Move the Bluetooth adapter or proxy closer to the AC.
- Set the scanner to **Active** or **Auto** mode.
- For a VM, confirm that its Bluetooth adapter is passed through.
- For Home Assistant Container, confirm that BlueZ and D-Bus are accessible.
- Confirm that a remote proxy supports active BLE connections and has a free
  connection slot.

## Debug logging

Enable debug logging for `custom_components.midea_ble` and inspect the Home
Assistant system log. Session keys and private keys are never logged.

## Development transaction probe

The integration and development probe share the same Python transaction client.
With the development dependencies installed, run:

```bash
python scripts/probe_python.py <BLE_ADDRESS> <ADVERTIS_DATA_HEX>
```

The default operation performs C1/C2/C3 authentication followed by a status
query. `--power-on` and `--power-off` preserve the reported settings, change only
the power state, and verify the returned status.

Home Assistant uses a separate dialer that selects the freshest connectable path
through its local adapters and supported Bluetooth proxies.

## Protocol documentation

- [BLE protocol specification](docs/protocol.md)
- [Python port and implementation notes](docs/protocol-port-plan.md)
- [Previously tested AC](docs/discovered-ac.md)

## Attribution and disclaimer

The protocol implementation was ported from the original
[`midea-ble-go`](https://github.com/sorinyang/midea-ble-go) research and reference
implementation.

The communication protocol was derived from publicly researched device
behavior; it is not an official or authorized Midea protocol implementation.
This project is intended for lawful interoperability, research, and personal use.
Users are responsible for ensuring that their use complies with applicable laws
and does not infringe third-party rights. The authors and contributors accept no
liability for consequences arising from use of this software.

Midea, Hualing, and their respective logos and trademarks belong to their owners.
Their use here is solely for product identification and does not imply
endorsement.

## License

The source code and protocol documentation are provided under the
[MIT License](LICENSE).
