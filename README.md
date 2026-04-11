# NextDDNS

[![Docker Build](https://github.com/monlor/NextDDNS/actions/workflows/docker.yml/badge.svg)](https://github.com/monlor/NextDDNS/actions/workflows/docker.yml)
[![GitHub Container Registry](https://img.shields.io/badge/ghcr.io-monlor%2Fnextddns-blue)](https://github.com/monlor/NextDDNS/pkgs/container/nextddns)

A Docker-friendly dynamic DNS sync service. Periodically resolves IPs from configurable sources and updates DNS records on Cloudflare and Tencent Cloud DNSPod.

## Features

- YAML-driven multi-task configuration
- IPv4-only, IPv6-only, or dual-stack sync per record
- Multiple IP sources:
  - `public` — probe public IP via HTTP (ipv4/ipv6)
  - `interface` — read from a local network interface
  - `dns` — resolve from an existing DNS hostname
  - `zte_star` — read device IPs from ZTE 星云 router (ZXSLC SR1010)
- DNS providers:
  - Cloudflare
  - Tencent Cloud DNSPod
- Health check endpoint at `/healthz`
- Environment variable substitution in config (`${VAR}`)

## Quick Start

```bash
docker run -d \
  --name nextddns \
  --restart unless-stopped \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/monlor/nextddns:latest \
  -config /app/config.yaml
```

### docker-compose

```yaml
services:
  nextddns:
    image: ghcr.io/monlor/nextddns:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    environment:
      - CF_API_TOKEN=xxx
      - TENCENT_SECRET_ID=xxx
      - TENCENT_SECRET_KEY=xxx
```

## Configuration

Copy `configs/config.example.yaml` and edit it. Credentials can be passed as environment variables using `${VAR}` syntax.

```yaml
server:
  listen: ":8080"

defaults:
  interval: 5m
  timeout: 10s
  log_format: text   # text | json

tasks:
  - name: my-task
    interval: 3m
    source:
      type: public          # public | interface | dns | zte_star
      ...
    providers:
      - type: cloudflare    # cloudflare | dnspod
        ...
        records:
          - zone: example.com
            name: home
            ttl: 300
            proxied: false
            ipv4: true
            ipv6: false
```

See [`configs/config.example.yaml`](configs/config.example.yaml) for a full example covering all source and provider types.

### IP Sources

| Type | Description |
|------|-------------|
| `public` | Probe public IP via configurable HTTP URLs |
| `interface` | Read from a named local network interface |
| `dns` | Resolve from an existing DNS hostname |
| `zte_star` | Read device table from ZTE 星云 router by MAC address |

### DNS Providers

| Type | Required fields |
|------|----------------|
| `cloudflare` | `api_token` |
| `dnspod` | `secret_id`, `secret_key` |

### Record options

| Field | Default | Description |
|-------|---------|-------------|
| `zone` | — | Root domain (e.g. `example.com`) |
| `name` | — | Subdomain (e.g. `home`) |
| `ttl` | `300` | TTL in seconds |
| `ipv4` | `true` | Sync A record |
| `ipv6` | `true` | Sync AAAA record |
| `proxied` | `false` | Cloudflare proxy (orange cloud) |

## Build

```bash
go build ./cmd/nextddns
./nextddns -config configs/config.example.yaml
```

```bash
docker build -t nextddns .
```

## Health Check

```
GET /healthz
```

Returns `200 OK` when the service is running.
