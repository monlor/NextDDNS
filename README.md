# NextDDNS

NextDDNS is a Docker-friendly dynamic DNS sync service with configurable IP sources. It can sync IPv4, IPv6, or both to Cloudflare and Tencent Cloud DNSPod using a YAML config file.

## Features

- YAML-driven task configuration
- IPv4-only, IPv6-only, or dual-stack sync
- IP sources:
  - specific interface address
  - public IP probes
  - existing DNS record lookup
  - configurable ZTE router backend adapter
- DNS providers:
  - Cloudflare
  - Tencent Cloud DNSPod
- Health check endpoint at `/healthz`

## Run

```bash
docker run --rm \
  -p 8080:8080 \
  -v $(pwd)/configs/config.example.yaml:/app/config.yaml:ro \
  -e CF_API_TOKEN=xxx \
  -e TENCENT_SECRET_ID=xxx \
  -e TENCENT_SECRET_KEY=xxx \
  ghcr.io/your-org/nextddns:latest \
  -config /app/config.yaml
```

## Build

```bash
go build ./cmd/nextddns
```

## Docker

```bash
docker build -t nextddns .
```
