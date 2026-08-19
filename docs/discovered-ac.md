# Discovered Midea AC

This device was discovered locally on 2026-08-19 using the repository's built
CLI:

```console
./build/midea-ble-go scan
```

## Device details

| Field | Value |
| --- | --- |
| Serial number | `00000Q16AC6982` |
| RSSI | `-61 dBm` |
| BLE identifier | `ab8e7547-1412-89c9-0e09-dbba9536d2ca` |
| Advertisement data | `ac3030303030513136607ad8916983` |

The 10-second scan completed successfully and identified the device as a Midea
air conditioner. Discovery confirms that the AC is advertising and recognized
by the CLI; it does not by itself confirm that handshake or control operations
succeed.
