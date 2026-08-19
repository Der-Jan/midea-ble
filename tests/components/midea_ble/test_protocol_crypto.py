"""Crypto interoperability and input-validation tests."""

import pytest

from custom_components.midea_ble.protocol.crypto import (
    cipher_message,
    create_keypair,
    decipher_message,
    derive_root_key,
    derive_session_key,
)
from custom_components.midea_ble.protocol.exceptions import MideaBleCryptoError


def test_root_key_upstream_go_vector() -> None:
    advertis_data = bytes.fromhex("ac3132333435363738aabbccddeeff")
    assert derive_root_key(advertis_data).hex() == "3ab82c346a77b6593d5ebe9f25d3cf50"


def test_p256_deterministic_public_and_session_key_vectors() -> None:
    private_a = (1).to_bytes(32, "big")
    private_b = (2).to_bytes(32, "big")
    _, public_a = create_keypair(private_key=private_a)
    _, public_b = create_keypair(private_key=private_b)
    assert public_a.hex() == (
        "6b17d1f2e12c4247f8bce6e563a440f277037d812deB33a0f4a13945d898c296"
        "4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5"
    ).lower()
    expected = "23775201799b2234a18e8071e409cec8"
    assert derive_session_key(private_a, public_b).hex() == expected
    assert derive_session_key(private_b, public_a).hex() == expected


def test_aes_ccm_fixed_vector() -> None:
    key = bytes.fromhex("3ab82c346a77b6593d5ebe9f25d3cf50")
    plaintext = bytes.fromhex("010706010203040506")
    blob = cipher_message(key, plaintext, random_bytes=lambda length: bytes(range(length)))
    assert blob.hex() == "000102030405060797c07c26039d4c8dbada45b521d9efccc4"
    assert decipher_message(key, blob) == plaintext


def test_aes_ccm_rejects_modified_tag() -> None:
    blob = cipher_message(bytes(16), b"hello", random_bytes=lambda length: bytes(length))
    corrupted = blob[:-1] + bytes([blob[-1] ^ 1])
    with pytest.raises(MideaBleCryptoError, match="authentication"):
        decipher_message(bytes(16), corrupted)


@pytest.mark.parametrize("key", [b"", bytes(15), bytes(17)])
def test_aes_ccm_rejects_invalid_key_length(key: bytes) -> None:
    with pytest.raises(MideaBleCryptoError):
        cipher_message(key, b"data")


def test_rejects_short_ciphertext() -> None:
    with pytest.raises(MideaBleCryptoError, match="too short"):
        decipher_message(bytes(16), bytes(15))
