# metal-apiserver

[![Actions](https://github.com/metal-stack/metal-apiserver/actions/workflows/build.yaml/badge.svg?branch=main)](https://github.com/metal-stack/metal-apiserver/actions)
[![codecov](https://codecov.io/gh/metal-stack/metal-apiserver/branch/main/graph/badge.svg)](https://codecov.io/gh/metal-stack/metal-apiserver)

The **metal-apiserver** is an implementation of the [metal-stack V2 API](https://github.com/metal-stack/api), the central part of the metal-stack control plane that manages the full lifecycle of physical servers, images, networking, IP addressing, and VPN connectivity for a bare-metal cloud.

## Overview

metal-apiserver provisions and manages bare-metal machines at scale. It handles:

- **Machine Lifecycle** - power on/off, boot image selection, provisioning, BMC management
- **Networking** - virtual networks, IPAM (prefix/IP allocation), VRF/NAT configuration
- **Multi-Tenancy** - projects and tenants with RBAC, isolation, and resource sharing
- **Machine Sizes** - size/flavor definitions with CPU, RAM, storage, and GPU constraints
- **OS Imaging** - image registry with versioning, classification (preview/supported/deprecated), and expiration
- **VPN** - WireGuard-based mesh VPN via Headscale (Tailscale-compatible control server)
- **Auditing** - optional Splunk HEC or TimescaleDB audit logging

## Architecture

```raw
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│   metalctl   │   │  Web Console │   │  Infra Agent │
│    (CLI)     │   │     (UI)     │   │ (metal-core) │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │                  │                  │
       └──────────────────┼──────────────────┘
                          │
              ┌───────────▼───────────┐
              │        HTTP/2         │
              │     (Connect-RPC)     │
              └───────────┬───────────┘
                          │
     ┌────────────────────▼────────────────────┐
     │         Interceptor Chain               │
     │  Metrics → Logger → AuthN → AuthZ →     │
     │  Rate-Limit → Validate → Tenant → Audit │
     └────────────────────┬────────────────────┘
                          │
     ┌────────────────────▼────────────────────┐
     │           Service Handlers              │
     │  ┌──────────┬──────────┬─────────────┐  │
     │  │ Public   │  Admin   │   Infra     │  │
     │  │ Services │ Services │  Services   │  │
     │  └──────────┴──────────┴─────────────┘  │
     └────────────────────┬────────────────────┘
                          │
     ┌────────────────────▼────────────────────┐
     │          Repository Layer               │
     │  (typed generics, validation, authZ)    │
     └────────────────────┬────────────────────┘
                          │
     ┌────────────────────▼────────────────────┐
     │             Storage Layer               │
     │                                         │
     │        ┌──────────┐  ┌──────────┐       │
     │        │RethinkDB │  │  Redis/  │       │
     │        │(primary) │  │  Valkey  │       │
     │        └──────────┘  └──────────┘       │
     │                                         │
     │               (external)                │
     │        ┌──────────┐  ┌──────────┐       │
     │        │  IPAM    │  │  Tenant  │       │
     │        │ (gRPC)   │  │  (gRPC)  │       │
     │        └──────────┘  └──────────┘       │
     │                                         │
     │                (optional)               │
     │        ┌──────────┐  ┌───────────┐      │
     │        │ Splunk / │  │ Headscale │      │
     │        │Timescale │  │   (VPN)   │      │
     │        └──────────┘  └───────────┘      │
     └─────────────────────────────────────────┘
```

### Layers

| Layer            | Responsibility                                                                                               |
|------------------|--------------------------------------------------------------------------------------------------------------|
| **Transport**    | HTTP/2 with gRPC compatibility, gRPC-Web support                                                             |
| **Interceptors** | Authentication (JWT/OIDC), authorization (RBAC), rate-limiting, validation, tenant resolution, audit logging |
| **Services**     | Public, admin, and infrastructure API handlers                                                               |
| **Repository**   | Business logic, data validation, authorization enforcement, protobuf conversion                              |
| **Storage**      | RethinkDB queries, domain models, migrations                                                                 |

### Storage Backends

| Backend                  | Purpose                                                                                                                    | Configuration                    |
|--------------------------|----------------------------------------------------------------------------------------------------------------------------|----------------------------------|
| **RethinkDB**            | Primary datastore -- machines, networks, IPs, sizes, images, partitions, switches, filesystem layouts, provisioning events | Required                         |
| **Redis/Valkey**         | Tokens (DB 0), rate-limiting (DB 1), invites (DB 2), async tasks (DB 3), component registry (DB 4)                         | Required                         |
| **go-ipam**              | IP Address Management -- prefix allocation                                                                                 | Required (external gRPC service) |
| **tenant-apiserver**     | Tenant/project membership                                                                                                  | Required (external gRPC service) |
| **TimescaleDB / Splunk** | Auditing                                                                                                                   | Optional                         |
| **Headscale**            | WireGuard VPN mesh control plane                                                                                           | Optional                         |

## End-User Guide

### Authentication

Authentication is performed via **OpenID Connect (OIDC)**. Three HTTP endpoints handle the login flow:

| Endpoint         | Method | Description                                       |
|------------------|--------|---------------------------------------------------|
| `/auth/login`    | GET    | Start OIDC authorization flow                     |
| `/auth/callback` | GET    | OIDC provider callback - exchanges code for token |
| `/auth/logout`   | GET    | Terminate session                                 |

**CLI login via `metalctl`:**

```bash
metalctl login --api-url <api-server-endpoint> --provider openid-connect
```

### Token Management

There are two token types:

| Type           | Purpose                                                                           |
|----------------|-----------------------------------------------------------------------------------|
| **User Token** | Created during OIDC login. Resolves roles dynamically from the tenant API server. |
| **API Token**  | Created programmatically. Contains self-contained static permissions in the JWT.  |

Create API tokens via the `TokenService/Create` API with role assignments:

```json
{
  "description": "ci-automation",
  "expires": "720h",
  "project_roles": {"my-project": "PROJECT_ROLE_EDITOR"},
  "tenant_roles": {"my-tenant": "TENANT_ROLE_VIEWER"}
}
```

See [RBAC.md](RBAC.md) for the complete role and permissions reference.

### Roles

| Scope                  | Roles                                                                                |
|------------------------|--------------------------------------------------------------------------------------|
| **Global (Admin)**     | `ADMIN_ROLE_EDITOR`, `ADMIN_ROLE_VIEWER`                                             |
| **Tenant**             | `TENANT_ROLE_OWNER`, `TENANT_ROLE_EDITOR`, `TENANT_ROLE_VIEWER`, `TENANT_ROLE_GUEST` |
| **Project**            | `PROJECT_ROLE_OWNER`, `PROJECT_ROLE_EDITOR`, `PROJECT_ROLE_VIEWER`                   |
| **Infrastructure**     | `INFRA_ROLE_EDITOR`, `INFRA_ROLE_VIEWER`                                             |
| **Machine (per UUID)** | `MACHINE_ROLE_EDITOR`, `MACHINE_ROLE_VIEWER`                                         |

## Developer Guide

### Prerequisites

- Go 1.26+
- RethinkDB
- Redis or Valkey
- (Optional) Docker for integration tests (via testcontainers-go)

### Building

```bash
make server        # Build the server binary to bin/server
make all           # fmt → test → server
```

The binary is statically linked with `CGO_ENABLED=1` and stripped.

### Running

```bash
bin/server serve
```

The server requires configured connections to RethinkDB, Redis, IPAM, and tenant API. Configuration is via environment variables and flags (see `cmd/server/serve.go`).

### CLI Commands

| Command                  | Description                                               |
|--------------------------|-----------------------------------------------------------|
| `serve`                  | Start the API server (HTTP + metrics + async task worker) |
| `token`                  | Create API tokens (for bootstrapping)                     |
| `datastore init`         | Initialize RethinkDB database (tables, indexes, pools)    |
| `datastore migrate`      | Run RethinkDB schema migrations                           |
| `vpn connected-machines` | Evaluate VPN-connected machines                           |

### DataStore Management

```bash
bin/server datastore init           # Initialize tables and indexes
bin/server datastore migrate        # Run pending migrations
bin/server datastore migrate --dry-run  # Preview migrations
```

### Testing

```bash
make test          # All tests with race detector, coverage
make bench         # Benchmarks
```

Tests use `testcontainers-go` for Postgres/Valkey containers. CI runs a matrix of test groups: `ip-network`, `project-tenant`, `machine-switch`, `partition-vpn`, `image-size`, `infra`, `e2e`, and `other`.

### Linting

```bash
make golint        # golangci-lint (bugs + unused groups)
```

### Project Structure

```raw
├── cmd/server/            # CLI entry point and sub-commands
├── pkg/
│   ├── async/            # Async task workflows and FIFO queues (asynq/Redis)
│   ├── auth/             # Connect-RPC interceptors (JWT auth, RBAC authorization)
│   ├── certs/            # X.509 certificate store for JWT signing (Redis)
│   ├── db/
│   │   ├── generic/      # Generic RethinkDB storage with typed Storage[E] interface
│   │   ├── metal/        # Domain entity models
│   │   └── queries/      # RethinkDB query builders per entity
│   ├── e2e/              # End-to-end integration tests
│   ├── fsm/              # Finite state machine for machine provisioning
│   ├── headscale/        # Headscale VPN client
│   ├── invite/           # Invitation store (Redis)
│   ├── issues/           # Machine issue detection
│   ├── k8s/              # Kubernetes secret helper
│   ├── rate-limiter/     # Rate limiting (Redis token bucket)
│   ├── repository/       # Business logic layer (typed CRUD, validation, authZ)
│   │   └── api/          # Internal API types
│   ├── request/          # Request authorization (role resolution from JWT)
│   ├── service/
│   │   ├── admin/        # Admin API handlers
│   │   ├── api/          # Public API handlers
│   │   ├── auth/         # OIDC auth HTTP handlers (login, callback, logout)
│   │   └── infra/        # Infra API handlers
│   ├── tags/             # Tag parsing utilities
│   ├── test/             # Test utilities
│   ├── token/            # JWT creation/validation, Redis token store
│   └── vpn/              # VPN connectivity evaluation
├── Makefile
├── Dockerfile
└── RBAC.md               # Complete RBAC and token management reference
```

### Key Design Patterns

- **Generic Repository** -- Go generics provide typed CRUD: `Repository[R Repo, M Message, C CreateMessage, U UpdateMessage, Q Query]`
- **Scoped Stores** -- `Store.IP(projectID)` returns a project-scoped repository; `Store.UnscopedIP()` returns admin-scoped
- **Interceptor Chain** -- Cross-cutting concerns (auth, logging, rate-limiting, validation) are Connect-RPC interceptors
- **Finite State Machine** -- Machine provisioning follows a state machine (`pkg/fsm/`)
- **Optimistic Locking** -- `generation`/`changed` timestamps prevent concurrent write conflicts
- **Async Workflows** -- Redis-backed `asynq` tasks for background processing

### Observability

| Feature              | Backend                                        |
|----------------------|------------------------------------------------|
| Metrics (Prometheus) | `:2112/metrics` via `prometheus/client_golang` |
| Tracing              | OpenTelemetry                                  |
| Audit Logging        | Optional Splunk HEC or TimescaleDB             |
| Health Endpoint      | `HealthService/Check` with sub-checkers        |

### Dependencies

Key Go dependencies:

- `connectrpc.com/connect` -- Connect-RPC framework
- `github.com/metal-stack/api` -- V2 API proto definitions
- `gopkg.in/rethinkdb/rethinkdb-go.v6` -- RethinkDB driver
- `github.com/redis/go-redis/v9` + `github.com/valkey-io/valkey-go` -- Redis/Valkey
- `github.com/hibiken/asynq` -- Async task queues
- `github.com/lestrrat-go/jwx/v3` -- JWT signing/verification
- `github.com/markbates/goth` (forked to `metal-stack/goth`) -- OIDC auth
- `github.com/looplab/fsm` -- Finite state machine
- `github.com/metal-stack/go-ipam` -- IPAM client
- `github.com/juanfont/headscale` -- Tailscale-compatible VPN control server

### Kubernetes Deployment

A `Dockerfile` produces a distroless static image. For development deployments, a `mini-lab` target is available:

```bash
make mini-lab-push     # Build, containerize, and deploy to kind cluster
```
