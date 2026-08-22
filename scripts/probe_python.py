#!/usr/bin/env python3
"""Exercise the integration client directly through a local Bleak adapter."""

import argparse
import asyncio
import logging
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from bleak import BleakClient  # noqa: E402

from custom_components.midea_ble.client import MideaBleClient  # noqa: E402
from custom_components.midea_ble.protocol.features import ACProperty  # noqa: E402
from custom_components.midea_ble.transport import BleakTransport  # noqa: E402


async def main_async(
    address: str,
    advertis_data: bytes,
    desired_power: bool | None,
    probe_features: bool,
    verify_features: bool,
    restore_vane_off: bool,
    verify_display: bool,
    verify_self_clean: bool,
    verify_presets: bool,
) -> None:
    """Use the same client as Home Assistant with a direct local dialer."""

    async def dial() -> BleakTransport:
        print(f"[*] Connecting to {address} ...")
        client = BleakClient(address, timeout=20.0)
        await client.connect()
        return BleakTransport(client)

    client = MideaBleClient(dial, advertis_data)
    if restore_vane_off:
        initial = await client.async_query()
        if not initial.power:
            await client.async_set_power(True)
        try:
            await client.async_set_swing(True, False)
            await client.async_set_swing(False, False)
            result = await client.async_probe_optional_features()
            print(
                "[+] Vertical vane after swing-on/off: "
                f"{result.value(ACProperty.WIND_UD_ANGLE)}"
            )
        finally:
            if not initial.power:
                await client.async_set_power(False)
        return
    if verify_display:
        initial_status = await client.async_query()
        if not initial_status.power:
            print("[*] Temporarily powering on for display verification")
            status = await client.async_set_power(True)
        else:
            status = initial_status
        original = status.screen_display
        print(f"[*] Display initial state: {original}")
        try:
            changed = await client.async_toggle_display()
            print(f"[+] Display after toggle: {changed.screen_display}")
            if changed.screen_display == original:
                raise RuntimeError("display toggle was not confirmed")
            restored = await client.async_toggle_display()
            print(f"[+] Display restored: {restored.screen_display}")
            if restored.screen_display != original:
                raise RuntimeError("display state was not restored")
        finally:
            if not initial_status.power:
                print("[*] Restoring original power-off state")
                await client.async_set_power(False)
        return
    if verify_self_clean:
        prop = ACProperty.SELF_CLEAN
        baseline = await client.async_probe_optional_features()
        original = baseline.value(prop)
        if original is None:
            raise RuntimeError("self-clean is not reported by this appliance")
        print(f"[*] Self-clean: {original} -> 1 -> {original}")
        try:
            observed = await client.async_set_optional_property(prop, 1)
            print(f"[+] Self-clean activation returned {observed}")
            if observed != 1:
                raise RuntimeError("self-clean activation was not confirmed")
        finally:
            restored = await client.async_set_optional_property(prop, original)
            print(f"[+] Self-clean restored: {restored}")
            if restored != original:
                raise RuntimeError("self-clean was not restored")
        return
    if verify_presets:
        initial_status = await client.async_query()
        if not initial_status.power:
            print("[*] Temporarily powering on for preset verification")
            await client.async_set_power(True)
        try:
            for preset, field in (
                ("sleep", "sleep_mode"),
                ("comfort", "comfort_mode"),
                ("away", "frost_protect"),
            ):
                print(f"[*] Testing {preset} preset")
                activated = await client.async_set_legacy_preset(preset, True)
                value = bool(getattr(activated, field))
                print(f"[+] {preset} activation reported {value}")
                restored = await client.async_set_legacy_preset(preset, False)
                restored_value = bool(getattr(restored, field))
                print(f"[+] {preset} restoration reported {restored_value}")
                if not value or restored_value:
                    print(f"[-] {preset} preset is unsupported; it will not be exposed")
        finally:
            if not initial_status.power:
                print("[*] Restoring original power-off state")
                await client.async_set_power(False)
        return
    if verify_features:
        initial_status = await client.async_query()
        if not initial_status.power:
            print("[*] Temporarily powering on for control verification")
            await client.async_set_power(True)
        try:
            baseline = await client.async_probe_optional_features()
            cases = (
                (ACProperty.RATE_SELECT, 80),
                (ACProperty.WIND_UD_ANGLE, 25),
                (ACProperty.OUTDOOR_SILENT, 0x03),
            )
            for prop, test_value in cases:
                original = baseline.value(prop)
                if original is None:
                    print(f"[-] {prop.name}: unsupported; skipped")
                    continue
                if original == test_value:
                    print(f"[-] {prop.name}: already {original}; skipped")
                    continue
                print(f"[*] {prop.name}: {original} -> {test_value} -> {original}")
                try:
                    observed = await client.async_set_optional_property(
                        prop, test_value
                    )
                    print(f"[+] set verification returned {observed}")
                    if observed != test_value:
                        print(
                            f"[-] {prop.name}: write was not confirmed; "
                            "feature will not be exposed"
                        )
                finally:
                    restored = await client.async_set_optional_property(prop, original)
                    if restored != original:
                        print(
                            f"[-] {prop.name}: firmware did not restore "
                            f"{original}; observed {restored}"
                        )
                    else:
                        print(f"[+] restored and confirmed {restored}")
        finally:
            if not initial_status.power:
                print("[*] Restoring original power-off state")
                await client.async_set_power(False)
        return
    if probe_features:
        result = await client.async_probe_optional_features()
        print("[+] B5 capabilities:")
        for tag, value in sorted(result.b5_values.items()):
            print(f"    0x{tag:04x}={value.hex()}")
        print("[+] Verified B1 properties:")
        for tag, value in sorted(result.values.items()):
            print(f"    {tag.name} (0x{tag:04x})={value.hex()}")
        refreshed = await client.async_query_data(result)
        if refreshed.optional is not None:
            print("[+] Combined supported-property refresh:")
            for tag, value in sorted(refreshed.optional.values.items()):
                print(f"    {tag.name}={value.hex()}")
        return
    if desired_power is None:
        data = await client.async_query_data()
        status = data.status
        print(
            f"[+] Python handshake/query succeeded: power={status.power} "
            f"mode={status.mode} temp={status.target_temperature} "
            f"fan={status.fan_speed}"
        )
        if data.energy is None:
            print("[-] AC did not return a supported C1/group-4 energy response")
        else:
            print(
                f"[+] Energy: realtime={data.energy.realtime_power} W "
                f"current={data.energy.current_energy_consumption} kWh "
                f"total={data.energy.total_energy_consumption} kWh"
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
    parser.add_argument("--debug-protocol", action="store_true")
    parser.add_argument("--probe-features", action="store_true")
    parser.add_argument("--verify-features", action="store_true")
    parser.add_argument("--restore-vane-off", action="store_true")
    parser.add_argument("--verify-display", action="store_true")
    parser.add_argument("--verify-self-clean", action="store_true")
    parser.add_argument("--verify-presets", action="store_true")
    args = parser.parse_args()
    if args.debug_protocol:
        handler = logging.StreamHandler()
        handler.setFormatter(logging.Formatter("[debug] %(message)s"))
        logger = logging.getLogger("custom_components.midea_ble.client")
        logger.addHandler(handler)
        logger.setLevel(logging.DEBUG)
    try:
        advertis_data = bytes.fromhex(args.advertis_data)
    except ValueError as err:
        parser.error(f"invalid advertis_data hex: {err}")
    desired_power = True if args.power_on else False if args.power_off else None
    asyncio.run(
        main_async(
            args.address,
            advertis_data,
            desired_power,
            args.probe_features,
            args.verify_features,
            args.restore_vane_off,
            args.verify_display,
            args.verify_self_clean,
            args.verify_presets,
        )
    )


if __name__ == "__main__":
    main()
