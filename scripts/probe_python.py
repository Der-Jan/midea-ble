#!/usr/bin/env python3
"""Exercise the integration client directly through a local Bleak adapter."""

import argparse
import asyncio
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from bleak import BleakClient  # noqa: E402

from custom_components.midea_ble.client import MideaBleClient  # noqa: E402
from custom_components.midea_ble.transport import BleakTransport  # noqa: E402


async def main_async(
    address: str, advertis_data: bytes, desired_power: bool | None
) -> None:
    """Use the same client as Home Assistant with a direct local dialer."""

    async def dial() -> BleakTransport:
        print(f"[*] Connecting to {address} ...")
        client = BleakClient(address, timeout=20.0)
        await client.connect()
        return BleakTransport(client)

    client = MideaBleClient(dial, advertis_data)
    if desired_power is None:
        status = await client.async_query()
        print(
            f"[+] Python handshake/query succeeded: power={status.power} "
            f"mode={status.mode} temp={status.target_temperature} "
            f"fan={status.fan_speed}"
        )
        return
    action = "on" if desired_power else "off"
    print(f"[*] Reading state and switching power {action} ...")
    status = await client.async_set_power(desired_power)
    if status.power is not desired_power:
        raise RuntimeError(f"device response did not confirm power {action}")
    print(
        f"[+] AC powered {action}: mode={status.mode} "
        f"temp={status.target_temperature} fan={status.fan_speed}"
    )


def main() -> None:
    """Parse command-line arguments and run the direct client."""
    parser = argparse.ArgumentParser()
    parser.add_argument("address", help="BLE address or macOS CoreBluetooth UUID")
    parser.add_argument("advertis_data", help="15-byte advertisData as hexadecimal")
    power = parser.add_mutually_exclusive_group()
    power.add_argument("--power-on", action="store_true")
    power.add_argument("--power-off", action="store_true")
    args = parser.parse_args()
    try:
        advertis_data = bytes.fromhex(args.advertis_data)
    except ValueError as err:
        parser.error(f"invalid advertis_data hex: {err}")
    desired_power = True if args.power_on else False if args.power_off else None
    asyncio.run(main_async(args.address, advertis_data, desired_power))


if __name__ == "__main__":
    main()
