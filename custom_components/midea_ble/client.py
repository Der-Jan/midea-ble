"""Async one-shot Midea BLE session client.

The protocol and retry behavior are ported from ``internal/ac/session.go``.
Connections are deliberately short-lived so Home Assistant Bluetooth proxies
and local adapters release their limited connection slots after each operation.
"""

from __future__ import annotations

import asyncio
import logging
import os
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Protocol

from .protocol.commands import (
    BIZ_TYPE_AC,
    ACState,
    build_control_frame,
    build_energy_query_frame,
    build_query_frame,
)
from .protocol.energy import ACEnergy, PowerAnalysisMethod, parse_energy_frame
from .protocol.exceptions import (
    MideaBleAuthenticationError,
    MideaBleConnectionError,
    MideaBleError,
    MideaBleHandshakeError,
)
from .protocol.features import (
    ACOptionalState,
    ACProperty,
    B5Capability,
    build_capability_query_frame,
    build_display_toggle_frame,
    build_property_query_frame,
    build_property_set_frame,
    parse_tlv_response,
)
from .protocol.frames import ConnFrameBuffer
from .protocol.handshake import HandshakeState, ReceiveInfo
from .protocol.status import ACStatus, parse_status_frame

_LOGGER = logging.getLogger(__name__)

NotifyCallback = Callable[[bytes], None]
StateMutation = Callable[[ACState], None]


@dataclass(frozen=True, slots=True)
class ACData:
    """One coordinated status and optional energy snapshot."""

    status: ACStatus
    energy: ACEnergy | None
    optional: ACOptionalState | None = None


class AsyncTransport(Protocol):
    """Minimal BLE byte transport required by the protocol session."""

    async def start_notify(self, callback: NotifyCallback) -> None:
        """Subscribe to incoming bytes."""

    async def write(self, data: bytes) -> None:
        """Write bytes with response."""

    async def close(self) -> None:
        """Unsubscribe and disconnect."""


Dialer = Callable[[], Awaitable[AsyncTransport]]


class _ConnectedSession:
    """One connected, authenticated protocol transaction."""

    def __init__(self, transport: AsyncTransport, advertis_data: bytes) -> None:
        self._transport = transport
        self._state = HandshakeState(advertis_data, os.urandom(6))
        self._queue: asyncio.Queue[ReceiveInfo | BaseException] = asyncio.Queue()
        self._reassembler = ConnFrameBuffer()

    def _on_notify(self, chunk: bytes) -> None:
        for raw in self._reassembler.feed(chunk):
            try:
                self._queue.put_nowait(self._state.on_receive(raw))
            except BaseException as err:
                self._queue.put_nowait(err)

    async def _wait_for(self, kind: str, timeout: float) -> ReceiveInfo:
        async with asyncio.timeout(timeout):
            while True:
                item = await self._queue.get()
                if isinstance(item, BaseException):
                    raise item
                if item.kind == "security_error":
                    raise MideaBleAuthenticationError(
                        f"device returned security error: {item.body.hex()}"
                    )
                if item.kind == kind:
                    return item

    async def _send_until_reply(
        self,
        kind: str,
        frame_factory: Callable[[], bytes],
        *,
        attempts: int,
        interval: float,
        reuse_frame: bool,
    ) -> ReceiveInfo:
        frame = frame_factory() if reuse_frame else None
        for _ in range(attempts):
            await self._transport.write(frame if frame is not None else frame_factory())
            try:
                return await self._wait_for(kind, interval)
            except TimeoutError:
                continue
        raise MideaBleConnectionError(f"timed out waiting for {kind}")

    async def handshake(self) -> None:
        """Subscribe and complete C1/C2/C3 authentication."""
        await self._transport.start_notify(self._on_notify)
        await asyncio.sleep(0.3)
        c1 = await self._send_until_reply(
            "c1",
            self._state.build_c1,
            attempts=15,
            interval=1.5,
            reuse_frame=False,
        )
        if c1.result == 1:
            raise MideaBleHandshakeError(
                "device requested a cached session, but no session key was restored"
            )
        if c1.result != 0:
            raise MideaBleAuthenticationError(f"unexpected C1 result: {c1.result}")
        c2 = await self._send_until_reply(
            "c2",
            self._state.build_c2,
            attempts=8,
            interval=1.2,
            reuse_frame=True,
        )
        if c2.peer_public_key is None:
            raise MideaBleHandshakeError("C2 response omitted the peer public key")
        self._state.establish_session(c2.peer_public_key)
        c3 = await self._send_until_reply(
            "c3",
            self._state.build_c3,
            attempts=8,
            interval=1.2,
            reuse_frame=True,
        )
        if c3.result != 1:
            raise MideaBleAuthenticationError(f"C3 result was {c3.result}")

    def _discard_pending(self) -> None:
        while not self._queue.empty():
            self._queue.get_nowait()

    async def business(
        self, appliance_frame: bytes, *, attempts: int = 4, timeout: float = 3.0
    ) -> bytes:
        """Send one appliance packet through encrypted BLE business frames."""
        self._discard_pending()
        for _ in range(attempts):
            frame = self._state.build_biz(BIZ_TYPE_AC, appliance_frame)
            await self._transport.write(frame)
            try:
                reply = await self._wait_for("biz", timeout)
            except TimeoutError:
                continue
            if reply.biz_body is None:
                raise MideaBleError("business response omitted its appliance frame")
            return reply.biz_body
        raise MideaBleConnectionError("timed out waiting for business response")


class MideaBleClient:
    """Serialize complete one-shot queries and read-modify-write controls."""

    def __init__(
        self,
        dialer: Dialer,
        advertis_data: bytes,
        power_analysis_method: PowerAnalysisMethod = PowerAnalysisMethod.PORTASPLIT,
    ) -> None:
        if len(advertis_data) < 11:
            raise ValueError("advertis_data must be at least 11 bytes")
        self._dialer = dialer
        self._advertis_data = bytes(advertis_data)
        self._operation_lock = asyncio.Lock()
        self._order = 0
        self._power_analysis_method = power_analysis_method

    def _next_order(self) -> int:
        self._order = self._order % 255 + 1
        return self._order

    async def _open(self) -> tuple[AsyncTransport, _ConnectedSession]:
        transport = await self._dialer()
        session = _ConnectedSession(transport, self._advertis_data)
        try:
            await session.handshake()
        except BaseException:
            await transport.close()
            raise
        return transport, session

    async def async_query(self) -> ACStatus:
        """Connect, authenticate, read status, and disconnect."""
        async with self._operation_lock:
            transport, session = await self._open()
            try:
                raw = await session.business(build_query_frame(self._next_order()))
                return parse_status_frame(raw)
            finally:
                await transport.close()

    async def async_query_data(
        self, optional: ACOptionalState | None = None
    ) -> ACData:
        """Read status and, when supported, energy in one encrypted session."""
        async with self._operation_lock:
            transport, session = await self._open()
            try:
                status_raw = await session.business(
                    build_query_frame(self._next_order())
                )
                status = parse_status_frame(status_raw)
                try:
                    energy_raw = await session.business(
                        build_energy_query_frame(), attempts=1
                    )
                    _LOGGER.debug(
                        "Received AC energy appliance frame: %s", energy_raw.hex()
                    )
                    energy = parse_energy_frame(
                        energy_raw, self._power_analysis_method
                    )
                except MideaBleError as err:
                    _LOGGER.debug("AC energy query is not supported: %s", err)
                    energy = None
                refreshed_optional = optional
                if optional and optional.values:
                    try:
                        raw = await session.business(
                            build_property_query_frame(
                                tuple(optional.values), self._next_order()
                            ),
                            attempts=1,
                        )
                        body_type, response = parse_tlv_response(raw)
                        if body_type == 0xB1:
                            values = {
                                prop: response[int(prop)]
                                for prop in optional.values
                                if int(prop) in response
                            }
                            refreshed_optional = ACOptionalState(
                                values=values,
                                b5_values=optional.b5_values,
                            )
                    except MideaBleError as err:
                        _LOGGER.debug("Optional property refresh failed: %s", err)
                return ACData(
                    status=status,
                    energy=energy,
                    optional=refreshed_optional,
                )
            finally:
                await transport.close()

    async def async_probe_optional_features(self) -> ACOptionalState:
        """Probe prioritized optional properties independently over BLE."""
        async with self._operation_lock:
            transport, session = await self._open()
            try:
                b5_values: dict[int, bytes] = {}
                for additional in (False, True):
                    try:
                        raw = await session.business(
                            build_capability_query_frame(
                                self._next_order(), additional=additional
                            ),
                            attempts=1,
                        )
                        _LOGGER.debug("Received AC B5 frame: %s", raw.hex())
                        body_type, b5_response = parse_tlv_response(raw)
                        if body_type == 0xB5:
                            b5_values.update(b5_response)
                    except MideaBleError as err:
                        _LOGGER.debug("B5 capability query failed: %s", err)

                properties = [
                    ACProperty.OUTDOOR_SILENT,
                    ACProperty.SCREEN_DISPLAY,
                    ACProperty.WIND_UD_ANGLE,
                    ACProperty.WIND_LR_ANGLE,
                    ACProperty.SELF_CLEAN,
                    ACProperty.INDOOR_HUMIDITY,
                    ACProperty.ERROR_CODE,
                ]
                electricity = b5_values.get(B5Capability.ELECTRICITY)
                if electricity and electricity[0] > 0:
                    properties.append(ACProperty.RATE_SELECT)

                property_values: dict[ACProperty, bytes] = {}
                for prop in properties:
                    try:
                        raw = await session.business(
                            build_property_query_frame(
                                (prop,), self._next_order()
                            ),
                            attempts=1,
                        )
                        _LOGGER.debug(
                            "Received AC property 0x%04x frame: %s",
                            prop,
                            raw.hex(),
                        )
                        body_type, response = parse_tlv_response(raw)
                        data = response.get(prop)
                        if body_type in (0xB0, 0xB1) and data is not None:
                            property_values[prop] = data
                    except MideaBleError as err:
                        _LOGGER.debug("Property 0x%04x query failed: %s", prop, err)
                return ACOptionalState(
                    values=property_values, b5_values=b5_values
                )
            finally:
                await transport.close()

    async def async_set_optional_property(
        self, prop: ACProperty, value: int
    ) -> int | None:
        """Set one B0 property, then query it back in the same BLE session."""
        async with self._operation_lock:
            transport, session = await self._open()
            try:
                set_raw = await session.business(
                    build_property_set_frame(prop, value, self._next_order())
                )
                _LOGGER.debug(
                    "Received AC property 0x%04x set response: %s",
                    prop,
                    set_raw.hex(),
                )
                query_raw = await session.business(
                    build_property_query_frame((prop,), self._next_order())
                )
                _LOGGER.debug(
                    "Received AC property 0x%04x verification: %s",
                    prop,
                    query_raw.hex(),
                )
                _, values = parse_tlv_response(query_raw)
                data = values.get(prop)
                return data[0] if data else None
            finally:
                await transport.close()

    async def async_toggle_display(self) -> ACStatus:
        """Toggle the legacy display control and return its C0 response."""
        async with self._operation_lock:
            transport, session = await self._open()
            try:
                raw = await session.business(
                    build_display_toggle_frame(self._next_order())
                )
                _LOGGER.debug("Received display-toggle response: %s", raw.hex())
                return parse_status_frame(raw)
            finally:
                await transport.close()

    async def async_control(self, mutate: StateMutation) -> ACStatus:
        """Connect, query current state, mutate it, write, verify, and disconnect."""
        async with self._operation_lock:
            transport, session = await self._open()
            try:
                current_raw = await session.business(
                    build_query_frame(self._next_order())
                )
                current = parse_status_frame(current_raw)
                state = current.as_control_state()
                mutate(state)
                result_raw = await session.business(
                    build_control_frame(state, self._next_order())
                )
                return parse_status_frame(result_raw)
            finally:
                await transport.close()

    async def async_set_power(self, power: bool) -> ACStatus:
        """Set power while preserving every reported setting."""

        def mutate(state: ACState) -> None:
            state.power = power

        return await self.async_control(mutate)

    async def async_set_mode(self, mode: int) -> ACStatus:
        """Set mode and power on."""

        def mutate(state: ACState) -> None:
            state.mode = mode
            state.power = True

        return await self.async_control(mutate)

    async def async_set_temperature(self, temperature: float) -> ACStatus:
        """Set both target-temperature fields."""

        def mutate(state: ACState) -> None:
            state.target_temperature = temperature
            state.target_temperature_2 = temperature

        return await self.async_control(mutate)

    async def async_set_fan_speed(self, fan_speed: int) -> ACStatus:
        """Set fan speed."""

        def mutate(state: ACState) -> None:
            state.fan_speed = fan_speed

        return await self.async_control(mutate)

    async def async_set_swing(self, up_down: bool, left_right: bool) -> ACStatus:
        """Set vertical and horizontal swing."""

        def mutate(state: ACState) -> None:
            state.swing_up_down = up_down
            state.swing_left_right = left_right

        return await self.async_control(mutate)

    async def async_set_eco(self, enabled: bool) -> ACStatus:
        """Set ECO mode."""

        def mutate(state: ACState) -> None:
            state.eco = enabled

        return await self.async_control(mutate)

    async def async_set_strong(self, enabled: bool) -> ACStatus:
        """Set strong/turbo mode."""

        def mutate(state: ACState) -> None:
            state.strong = enabled

        return await self.async_control(mutate)

    async def async_set_legacy_preset(self, preset: str, enabled: bool) -> ACStatus:
        """Set a C0-backed sleep, comfort, or away/frost-protect preset."""
        if preset not in {"sleep", "comfort", "away"}:
            raise ValueError(f"unknown AC preset: {preset}")

        def mutate(state: ACState) -> None:
            state.sleep_mode = enabled if preset == "sleep" else False
            state.comfort_mode = enabled if preset == "comfort" else False
            state.frost_protect = enabled if preset == "away" else False

        return await self.async_control(mutate)
