"""End-to-end async client tests against an in-memory AC simulator."""

import asyncio

from custom_components.midea_ble.client import MideaBleClient, NotifyCallback
from custom_components.midea_ble.protocol.commands import (
    BIZ_TYPE_AC,
    _checksum,
    _crc8_854,
)
from custom_components.midea_ble.protocol.crypto import (
    cipher_message,
    create_keypair,
    decipher_message,
    derive_root_key,
    derive_session_key,
)
from custom_components.midea_ble.protocol.features import ACProperty
from custom_components.midea_ble.protocol.frames import (
    CONN_T2,
    CONN_T3,
    SEC_C1,
    SEC_C2,
    SEC_C3,
    SEC_C4,
    decode_biz,
    decode_conn,
    decode_security,
    encode_biz,
    encode_conn,
    encode_security,
)

ADVERTIS_DATA = bytes.fromhex("ac3132333435363738aabbccddeeff")


class FakeACTransport:
    """Act like the device side of C1/C2/C3 and basic C4 status/control."""

    def __init__(self) -> None:
        self.root_key = derive_root_key(ADVERTIS_DATA)
        self.device_private, self.device_public = create_keypair(
            private_key=(2).to_bytes(32, "big")
        )
        self.session_key: bytes | None = None
        self.callback: NotifyCallback | None = None
        self.sequence = 0
        self.power = False
        self.closed = False
        self.appliance_requests: list[bytes] = []

    async def start_notify(self, callback: NotifyCallback) -> None:
        self.callback = callback

    def _next_sequence(self) -> int:
        self.sequence = self.sequence % 255 + 1
        return self.sequence

    def _respond(self, frame_type: int, key: bytes, command: int, body: bytes) -> None:
        security = encode_security(command, body, self._next_sequence())
        raw = encode_conn(
            frame_type,
            cipher_message(key, security),
            self._next_sequence(),
        )
        assert self.callback is not None
        # Exercise fragmented notification reassembly.
        self.callback(raw[:4])
        self.callback(raw[4:])

    def _status(self) -> bytes:
        raw = bytearray.fromhex(
            "aa2aac00000000000803c0004a667f7f0000000000676c0e0037000000000000"
            "000000000000b7ff424c00"
        )
        raw[11] = int(self.power)
        raw[-1] = -sum(raw[1:-1]) & 0xFF
        return bytes(raw)

    def _property_response(self, tag: int, value: int | None) -> bytes:
        body = bytearray((0xB1, 1, tag & 0xFF, tag >> 8, 0, int(value is not None)))
        if value is not None:
            body.append(value)
        body.append(0)
        body.append(_crc8_854(bytes(body)))
        frame = bytearray(10 + len(body) + 1)
        frame[:10] = bytes.fromhex("aa00ac00000000000803")
        frame[1] = len(frame) - 1
        frame[10:-1] = body
        frame[-1] = _checksum(bytes(frame[1:-1]))
        return bytes(frame)

    async def write(self, data: bytes) -> None:
        conn = decode_conn(data)
        if conn.frame_type == CONN_T2:
            security = decode_security(decipher_message(self.root_key, conn.body))
            if security.command == SEC_C1:
                self._respond(CONN_T2, self.root_key, SEC_C1, b"\x00")
            elif security.command == SEC_C2:
                self._respond(CONN_T2, self.root_key, SEC_C2, self.device_public)
            elif security.command == SEC_C3:
                peer_public = security.body[:64]
                self.session_key = derive_session_key(self.device_private, peer_public)
                assert decipher_message(self.session_key, security.body[64:]) == ADVERTIS_DATA
                self._respond(CONN_T2, self.root_key, SEC_C3, b"\x01")
            return
        assert conn.frame_type == CONN_T3
        assert self.session_key is not None
        security = decode_security(decipher_message(self.session_key, conn.body))
        assert security.command == SEC_C4
        biz = decode_biz(security.body)
        assert biz.frame_type == BIZ_TYPE_AC
        self.appliance_requests.append(biz.body)
        if biz.body[10] == 0xB5:
            response = bytes.fromhex(
                "aa3dac00000000000803b50a1202010114020101150201001e02010117020102"
                "1a02010110020101250207203c203c203c0024020101480001010101c71a"
                if biz.body[12] == 0
                else "aa2fac00000000000803b5081f0201002c020101160201043900010151000101"
                "e300010113020101cd0001030002365b"
            )
            self._respond(
                CONN_T3,
                self.session_key,
                SEC_C4,
                encode_biz(BIZ_TYPE_AC, response),
            )
            return
        if biz.body[10] == 0xB1:
            tag = biz.body[12] | (biz.body[13] << 8)
            values = {
                ACProperty.OUTDOOR_SILENT: 0,
                ACProperty.WIND_UD_ANGLE: 0,
                ACProperty.SELF_CLEAN: 0,
                ACProperty.RATE_SELECT: 100,
            }
            response = self._property_response(tag, values.get(ACProperty(tag)))
            self._respond(
                CONN_T3,
                self.session_key,
                SEC_C4,
                encode_biz(BIZ_TYPE_AC, response),
            )
            return
        if biz.body == bytes.fromhex(
            "aa11ac00000000000003412101440001098f"
        ):
            response = bytes.fromhex(
                "aa22ac00000000000803c121014400000017000000000000000000000f"
                "000000015d7c"
            )
            self._respond(
                CONN_T3,
                self.session_key,
                SEC_C4,
                encode_biz(BIZ_TYPE_AC, response),
            )
            return
        if biz.body[10] == 0x40:
            self.power = bool(biz.body[11] & 1)
        self._respond(
            CONN_T3,
            self.session_key,
            SEC_C4,
            encode_biz(BIZ_TYPE_AC, self._status()),
        )

    async def close(self) -> None:
        self.closed = True


class FakeDialer:
    def __init__(self) -> None:
        self.transports: list[FakeACTransport] = []
        self.power = False

    async def __call__(self) -> FakeACTransport:
        transport = FakeACTransport()
        transport.power = self.power
        original_close = transport.close

        async def close() -> None:
            self.power = transport.power
            await original_close()

        transport.close = close  # type: ignore[method-assign]
        self.transports.append(transport)
        return transport


def test_query_and_power_read_modify_write() -> None:
    async def run() -> None:
        dialer = FakeDialer()
        client = MideaBleClient(dialer, ADVERTIS_DATA)
        initial = await client.async_query()
        assert not initial.power
        updated = await client.async_set_power(True)
        assert updated.power
        verified = await client.async_query()
        assert verified.power
        assert len(dialer.transports) == 3
        assert all(transport.closed for transport in dialer.transports)

    asyncio.run(run())


def test_energy_query_is_carried_inside_encrypted_ble_business_frame() -> None:
    async def run() -> None:
        dialer = FakeDialer()
        client = MideaBleClient(dialer, ADVERTIS_DATA)
        data = await client.async_query_data()
        assert data.energy is not None
        assert data.energy.realtime_power == 1.5
        assert data.energy.current_energy_consumption == 0.0
        assert data.energy.total_energy_consumption == 0.23
        transport = dialer.transports[0]
        assert transport.appliance_requests[1].hex() == (
            "aa11ac00000000000003412101440001098f"
        )
        assert transport.closed

    asyncio.run(run())


def test_optional_capability_queries_use_encrypted_ble_business_frames() -> None:
    async def run() -> None:
        dialer = FakeDialer()
        client = MideaBleClient(dialer, ADVERTIS_DATA)
        optional = await client.async_probe_optional_features()
        assert optional.value(ACProperty.OUTDOOR_SILENT) == 0
        assert optional.value(ACProperty.RATE_SELECT) == 100
        assert optional.value(ACProperty.WIND_UD_ANGLE) == 0
        assert ACProperty.WIND_LR_ANGLE not in optional.values
        requests = dialer.transports[0].appliance_requests
        assert requests[0].hex() == "aa0fac00000000000803b5010001e59e"
        assert all(request.startswith(b"\xaa") for request in requests)
        assert dialer.transports[0].closed

    asyncio.run(run())
