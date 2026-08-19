"""Deterministic handshake-message tests."""

from custom_components.midea_ble.protocol.crypto import cipher_message
from custom_components.midea_ble.protocol.frames import (
    CONN_T2,
    SEC_C1,
    decode_conn,
    decode_security,
)
from custom_components.midea_ble.protocol.handshake import HandshakeState


class DeterministicRandom:
    """Return predictable bytes while honoring each requested length."""

    def __init__(self) -> None:
        self.value = 0

    def __call__(self, length: int) -> bytes:
        result = bytes((self.value + index) & 0xFF for index in range(length))
        self.value = (self.value + length) & 0xFF
        return result


def test_build_c1_roundtrip() -> None:
    state = HandshakeState(
        bytes.fromhex("ac3132333435363738aabbccddeeff"),
        bytes.fromhex("010203040506"),
        random_bytes=DeterministicRandom(),
    )
    conn = decode_conn(state.build_c1())
    assert conn.frame_type == CONN_T2
    from custom_components.midea_ble.protocol.crypto import decipher_message

    security = decode_security(decipher_message(state.root_key, conn.body))
    assert security.command == SEC_C1
    assert security.body == bytes.fromhex("010203040506")


def test_parse_c1_response() -> None:
    state = HandshakeState(
        bytes.fromhex("ac3132333435363738aabbccddeeff"),
        bytes.fromhex("010203040506"),
    )
    security = bytes((SEC_C1, 7, 1, 0))
    encrypted = cipher_message(state.root_key, security)
    from custom_components.midea_ble.protocol.frames import encode_conn

    info = state.on_receive(encode_conn(CONN_T2, encrypted, 9))
    assert info.kind == "c1"
    assert info.result == 0
