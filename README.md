# miscale2-tap

A passive BLE weight collector for Xiaomi Mi Body Composition Scale 2 (XMTZC05HM) on ESP32-C6.

```
Scale ── BLE advertise ──▶ ESP32-C6 ── HTTP POST ──▶ Go + SQLite
XMTZC05HM                  ESPHome
```

## Prerequisites

- ESP32-C6
- Xiaomi Mi Body Composition Scale 2 (XMTZC05HM)
- ESPHome CLI (`brew install esphome`)
- Docker

## Quick Start

### Firmware

```bash
cp firmware/secrets.yaml.example firmware/secrets.yaml
# fill in secrets.yaml

esphome run firmware/scale-bridge.yaml
```

> First flash must be USB (BLE tracker triggers partition resize). After that, use OTA.

### Ingest Server

```bash
cd ingest
docker build --platform linux/amd64 -t miscale2-tap:latest .

docker run -d --name miscale2-tap --restart unless-stopped \
  -p 51001:51001 \
  -v $(pwd)/data:/data \
  -e DB_PATH=/data/weight.db \
  -e LISTEN_ADDR=:51001 \
  -e API_TOKEN= \
  -e CALLBACK_URL= \
  miscale2-tap:latest
```

## Configuration

### Firmware Secrets (`firmware/secrets.yaml`)

| Key | Description |
|---|---|
| `wifi_ssid` | WiFi SSID |
| `wifi_password` | WiFi password |
| `api_key` | ESPHome API encryption key (`openssl rand -base64 32`) |
| `ota_password` | OTA update password (`openssl rand -hex 16`) |
| `scale_mac` | Scale BLE MAC address (from Zepp Life → My devices) |
| `ingest_url` | `http://<server-ip>:51001/weight` |

### Ingest Server Environment Variables

| Name | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:51001` | Server bind address |
| `DB_PATH` | `/data/weight.db` | SQLite database path |
| `API_TOKEN` | | Bearer token for read endpoints and `POST /weight/clear`. Open when unset. |
| `CALLBACK_URL` | | POSTs weight JSON on each new measurement. Disabled when unset. |

## API

| Endpoint | Description |
|---|---|
| `POST /weight` | `{"kg": 73.45, "ohm": 512}` → `204`. `ohm` optional. |
| `GET /weight?from=&to=&limit=&order=` | JSON list (default 100, max 1000) |
| `GET /weight/latest` | Latest record or `404` |
| `GET /weight.csv?from=&to=` | CSV export |
| `GET /healthz` | DB check |
| `POST /weight/clear` | Delete all records |

`from`/`to` accept `YYYY-MM-DD` or RFC 3339. Timestamps are UTC. Impedance is `null` when absent.

## BLE Protocol

Service UUID `0x181B`, 13 bytes:

```
[0]      unit         0x02 = kg
[1]      flags        bit 1: has_impedance, bit 5: stabilized, bit 7: load_removed
[2–8]    timestamp    ignored (server stamps its own)
[9–10]   impedance    LE uint16 (0 or ≥3000 → invalid)
[11–12]  weight       LE uint16 (kg = raw / 200)
```

Only `stabilized && !load_removed` packets are accepted. Range: 10–200 kg. Scale unit must be kg.

## Diagnostics

Uncomment `on_ble_advertise` in `scale-bridge.yaml` to log raw 13-byte service data hex.
