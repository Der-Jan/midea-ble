# Optional PortaSplit appliance commands

These commands are complete inner Midea AC `AA...` packets. LAN integrations
wrap them in Midea's LAN protocol; this integration instead wraps them in BLE
business type `0x20`, security command C4, AES-CCM, and connection type T3.
The appliance protocol byte for B0/B1/B5 packets is `0x08`.

The message-id byte varies per request, so the final body CRC and appliance
checksum vary with it as well.

| Feature | Inner request/body | Response/status | Tested PortaSplit result |
| --- | --- | --- | --- |
| B5 capabilities | `B5 01 00 ID CRC`; additional page `B5 01 01 01 ID CRC` | B5 four-byte-header TLVs | Supported; both pages returned |
| Outdoor Silent | B1 query tag `CD 00`; B0 set TLV `CD 00 01 03/00` | B1 tag `0x00CD`, `03` on, `00` off | Control verified on and restored off |
| Power rate/gear | B1 tag `48 00`; B0 value `01/20/40/60/80/100` | B1 tag `0x0048`; B5 `0x0216 > 0` gates support | 100→80→100 verified |
| Screen display | `41 02 00 FF 02 00 02` plus zero fill, ID and CRC | C0 byte 14 display bits; B5 tag `0x0224` gates support | Toggle off/on verified and restored |
| Vertical vane | B1/B0 tag `09 00`, positions `1/25/50/75/100` | B1 tag `0x0009`; B5 `0x0215` | Position 25 verified; zero requires a swing-on/off cycle rather than B0, so it is not exposed |
| Horizontal vane | B1/B0 tag `0A 00` | B1 tag `0x000A`; B5 `0x0215` | No value returned; not exposed |
| Sleep | Full-state `0x40`; body byte 10 bit `0x01` | C0 body byte 10 bit `0x01` | Activation and restoration verified, but B5 has no capability flag; not exposed |
| Comfort | Full-state `0x40`; body byte 22 bit `0x01` | C0 body byte 22 bit `0x01` | Rejected; not exposed |
| Away/frost | Full-state `0x40`; body byte 21 bit `0x80` | C0 body byte 21 bit `0x80` | Rejected; not exposed |
| Self-clean | B1 tag `39 00`; B0 value `01/00` | B1 tag `0x0039`; B5 tag `0x0039` gates support | Activation/cancellation verified |
| Indoor humidity | B1 tag `15 00`, or C1 group query `41 21 01 45 00 01 A2` | B1 `0x0015` or C1 group `0x45`, byte 4 | B5 reports unsupported and B1 returned no value; not exposed |
| Error code | B1 tag `3F 00` | B1 tag `0x003F` | No value returned; not exposed |

The B0 set encoding uses four-byte TLV headers (`tag-low`, `tag-high`,
`length`, `value`). B1 responses use a fixed padding byte before `length`; B5
responses omit that padding. Confusing these layouts causes a syntactically
valid packet whose write is ignored, so each codec is kept distinct.
