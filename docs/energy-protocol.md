# AC power and energy protocol

The BLE integration reuses Midea's appliance-level AC protocol for power and
energy data. It does **not** reuse the LAN transport framing.

## Appliance request and response

`midea-local` and `midea_ac_lan` issue the AC group-data query with body:

```text
41 21 01 44 00 01 09
```

The last byte is the appliance-body CRC. With the ten-byte V1 AC header and
outer appliance checksum, the packet used by this integration is:

```text
AA 11 AC 00 00 00 00 00 00 03 41 21 01 44 00 01 09 8F
```

The reply is a query response (`frame[9] == 0x03`) whose body type/opcode is
`0xC1`. Group byte `body[3] == 0x44` identifies the power/energy response:

| C1 body bytes | Meaning |
| --- | --- |
| `4..7` | Total energy consumption |
| `8..11` | Total operating consumption (not exposed) |
| `12..15` | Current energy consumption |
| `16..18` | Realtime power |

For PortaSplit analysis method 12, each field is an unsigned big-endian binary
integer. Energy values are divided by 100 and reported in kWh; realtime power
is divided by 10 and reported in W. Method 12 therefore differs from method 2
only in energy resolution: method 2 divides energy by 10.

## BLE transport verification

The complete `AA...` packet is passed as the body of BLE business type `0x20`,
then encoded in security command C4, encrypted/authenticated with the negotiated
AES-CCM session key, and finally placed in a BLE connection type-T3 frame. The
reply takes the reverse path before the `C1` appliance packet is parsed.

Golden tests cover a published PortaSplit `00000Q1D` subtype-524 capture and a
capture obtained through an encrypted BLE session from `00000Q16AC6982`. An
encrypted in-memory C1/C2/C3/C4 exchange inspects the decrypted BLE business
request and returns that BLE capture, verifying the transport boundary
independently from the appliance codec.

The integration currently selects analysis method 12. Other Midea models may
use methods 1, 2, 3, or 101; their decoders are implemented, but model-specific
selection is not yet exposed in the config flow. Sensors are created only when
the appliance answers the initial group-4 query with a valid C1/group-44 frame.
