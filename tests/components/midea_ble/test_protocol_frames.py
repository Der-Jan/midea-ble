"""Frame-codec tests ported from internal/proto/frame.go."""

from collections.abc import Callable

import pytest

from custom_components.midea_ble.protocol.exceptions import MideaBleFrameError
from custom_components.midea_ble.protocol.frames import (
    CONN_T1,
    SEC_C2,
    ConnFrameBuffer,
    decode_biz,
    decode_conn,
    decode_security,
    encode_biz,
    encode_conn,
    encode_security,
)


def test_conn_roundtrip_and_fixed_vector() -> None:
    raw = encode_conn(CONN_T1, bytes((1, *([0] * 9))), 7)
    assert raw.hex() == "aa550e070101000000000000000000e9"
    decoded = decode_conn(raw)
    assert decoded.frame_type == CONN_T1
    assert decoded.sequence == 7
    assert decoded.body == bytes((1, *([0] * 9)))


def test_security_roundtrip() -> None:
    raw = encode_security(SEC_C2, b"\x01\x02\x03", 3)
    decoded = decode_security(raw)
    assert decoded.command == SEC_C2
    assert decoded.sequence == 3
    assert decoded.body == b"\x01\x02\x03"


def test_biz_roundtrip() -> None:
    decoded = decode_biz(encode_biz(0x20, b"payload"))
    assert decoded.frame_type == 0x20
    assert decoded.body == b"payload"


def test_biz_accepts_real_device_style_nonzero_checksum() -> None:
    raw = bytearray(encode_biz(0x20, b"payload"))
    raw[-1] ^= 1
    assert decode_biz(bytes(raw)).body == b"payload"


def test_conn_reassembly_handles_noise_fragments_and_multiple_frames() -> None:
    first = encode_conn(CONN_T1, b"one", 1)
    second = encode_conn(CONN_T1, b"two", 2)
    buffer = ConnFrameBuffer()
    assert buffer.feed(b"noise" + first[:4]) == []
    assert buffer.feed(first[4:] + second) == [first, second]


@pytest.mark.parametrize(
    "mutate",
    [
        lambda raw: raw[:-1],
        lambda raw: raw + b"\x00",
        lambda raw: raw[:-1] + bytes((raw[-1] ^ 1,)),
    ],
)
def test_conn_rejects_invalid_length_or_checksum(
    mutate: Callable[[bytes], bytes],
) -> None:
    with pytest.raises(MideaBleFrameError):
        decode_conn(mutate(encode_conn(CONN_T1, b"body", 1)))
