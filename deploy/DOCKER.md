# Sub2API Docker Image

Sub2API is an AI API Gateway Platform for distributing and managing AI product subscription API quotas.

## Quick Start

```bash
docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/sub2api" \
  s2644752646/sub2api:latest
```

By default, the image auto-starts an embedded Redis instance when `REDIS_URL`
is unset and `REDIS_HOST` is local.

## Docker Compose

```yaml
version: '3.8'

services:
  sub2api:
    image: s2644752646/sub2api:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://postgres:postgres@db:5432/sub2api?sslmode=disable
      - REDIS_HOST=127.0.0.1
      - LOCAL_REDIS_ENABLED=true
    depends_on:
      - db

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=sub2api
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Yes | - |
| `REDIS_URL` | External Redis connection string | No | - |
| `REDIS_HOST` | Redis host for embedded/local mode | No | `127.0.0.1` |
| `LOCAL_REDIS_ENABLED` | Auto-start embedded Redis when no `REDIS_URL` is set | No | `true` |
| `LOCAL_REDIS_MAXMEMORY` | Embedded Redis maxmemory | No | `128mb` |
| `PORT` | Server port | No | `8080` |
| `GIN_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `latest` - Latest stable release
- `x.y.z` - Specific version
- `x.y` - Latest patch of minor version
- `x` - Latest minor of major version

## Links

- [GitHub Repository](https://github.com/sh2001sh/sub2api)
- [Documentation](https://github.com/sh2001sh/sub2api#readme)
