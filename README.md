# metal-apiserver

[![Actions](https://github.com/metal-stack/metal-apiserver/actions/workflows/build.yaml/badge.svg?branch=main)](https://github.com/metal-stack/metal-apiserver/actions)
[![Slack](https://img.shields.io/badge/slack-metal--stack-brightgreen.svg?logo=slack)](https://metal-stack.slack.com/)
[![codecov](https://codecov.io/gh/metal-stack/metal-apiserver/branch/main/graph/badge.svg)](https://codecov.io/gh/metal-stack/metal-apiserver)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/metal-stack/metal-apiserver)

The metal-apiserver is the central component of the metal-stack control plane. It implements the metal-stack [v2 API](https://github.com/metal-stack/api) and is both the public interface for users to manage machines, networks, IPs, images, sizes and so forth, and the interface for metal-stack components running inside a partition (metal-core, metal-bmc, metal-hammer, switch agents, ...).

Starting with the metal-stack v1.0.0 release, the metal-apiserver is the default API of metal-stack and supersedes the [metal-api](https://github.com/metal-stack/metal-api).

The CLI for using the API is `metalctlv2`, which lives in [metal-stack/cli](https://github.com/metal-stack/cli). Client libraries for Go and other languages are generated from the API definitions in [metal-stack/api](https://github.com/metal-stack/api).

## Features

- **[Connect](https://connectrpc.com/) based API**: every service can be consumed via the Connect protocol, gRPC and gRPC-Web over HTTP/1.1 and HTTP/2 (including h2c). Request validation is done with [protovalidate](https://github.com/bufbuild/protovalidate), gRPC health checking and server reflection are available.
- **Three API surfaces**:
  - `api` – services for tenants and projects (machines, IPs, networks, images, tokens, members, ...)
  - `admin` – services for operators of a metal-stack installation
  - `infra` – services used by metal-stack components inside a partition (boot, BMC, switches, events, components)
- **Authentication via OIDC** for human users and **JWT based API tokens** for automation, with fine-grained, role- and permission-based authorization. See [RBAC.md](RBAC.md) for all details.
- **Multi-tenancy** backed by the [tenant-apiserver](https://github.com/metal-stack/tenant-apiserver), including tenant and project memberships and invites.
- **IP address management** through [go-ipam](https://github.com/metal-stack/go-ipam).
- **Asynchronous task processing** (e.g. BMC commands, cross-backend transactions) based on [asynq](https://github.com/hibiken/asynq), see [pkg/async](pkg/async/README.md).
- **Machine lifecycle tracking** with a finite state machine and automatic machine issue detection.
- **Optional VPN access** to machines via [headscale](https://github.com/juanfont/headscale).
- **Auditing** to TimescaleDB and/or Splunk.
- **Rate limiting** per token and for unauthenticated access.
- **Observability**: structured logging, Prometheus metrics and OpenTelemetry instrumentation.

## Architecture

```mermaid
flowchart LR
    cli[metalctlv2 / UI / clients] -->|Connect / gRPC| apiserver
    partition[partition components<br/>metal-core, metal-bmc, ...] -->|Connect / gRPC| apiserver
    oidc[OIDC provider] <-->|login| apiserver

    subgraph control-plane
      apiserver[metal-apiserver]
      apiserver --> rethinkdb[(RethinkDB)]
      apiserver --> valkey[(Redis / Valkey)]
      apiserver --> ipam[go-ipam]
      apiserver --> tenant[tenant-apiserver]
      apiserver -.-> headscale[headscale]
      apiserver -.-> audit[(TimescaleDB / Splunk)]
    end
```

| Dependency                                                       | Required | Purpose                                                       |
| ---------------------------------------------------------------- | -------- | ------------------------------------------------------------- |
| [RethinkDB](https://rethinkdb.com/)                              | yes      | Primary datastore for all metal entities                     |
| Redis / [Valkey](https://valkey.io/)                             | yes      | Token store, rate limiting, async task queues                |
| [go-ipam](https://github.com/metal-stack/go-ipam)                | yes      | IP address and prefix management                              |
| [tenant-apiserver](https://github.com/metal-stack/tenant-apiserver) | yes   | Tenants, projects and their memberships                       |
| OIDC provider                                                    | yes      | User login (e.g. Zitadel, Keycloak, Dex, GitHub, ...)         |
| [headscale](https://github.com/juanfont/headscale)               | no       | VPN access to machines                                        |
| TimescaleDB / Splunk                                             | no       | Audit trail                                                   |

## Deployment

The metal-apiserver is deployed as part of the metal-stack control plane with the [metal-roles](https://github.com/metal-stack/metal-roles) Ansible roles. Container images are built for every release.

For a local, fully functional environment including the metal-apiserver, use the [mini-lab](https://github.com/metal-stack/mini-lab). Please refer to the [metal-stack documentation](https://metal-stack.io) for an overview of all components.

## Development

Requirements: Go (see [go.mod](go.mod)) and Docker, as the tests start their backends (RethinkDB, Valkey, go-ipam, headscale, ...) with [testcontainers](https://testcontainers.com/).

```bash
make server   # build bin/server
make test     # run all tests with coverage
make golint   # run golangci-lint
make bench    # run benchmarks
```

To test changes in a running [mini-lab](https://github.com/metal-stack/mini-lab) (checked out next to this repository), build and deploy your local version with:

```bash
make mini-lab-push
```

## License

This project is licensed under the [MIT License](LICENSE).
