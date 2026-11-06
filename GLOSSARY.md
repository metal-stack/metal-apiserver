# Glossary

The domain language of the metal-apiserver. One term per concept; no implementation detail.

## Entity

A metal-stack resource as the apiserver stores and manipulates it internally — Machine,
Network, IP, Switch, Partition, Size, Image, FilesystemLayout, SizeReservation,
SizeImageConstraint, Event. Distinct from the API representation of the same resource.
_Avoid_: model, record, document.

## Message

The API-facing representation of an Entity that consumers send and receive. An Entity and
its Message describe the same resource at different edges of the apiserver.
_Avoid_: DTO, proto (the proto is one Message implementation, not the concept).

## Scope

The boundary a Repository acts within. A scoped Repository only sees resources inside its
boundary; an unscoped Repository sees all of them.
_Avoid_: context (reserved for Go's `context.Context`), tenant.

## Repository

The module that owns the operations for one Entity: validation, scope matching, the CRUD
orchestration, and the translation between Entity and Message. A Repository is always
created for a specific Scope.

## Additional Methods

Operations a Repository exposes beyond basic CRUD — the entity-specific business logic.

## Entity Storage

The seam through which a Repository persists and retrieves Entities, independent of which
Backend serves it. Entity reads and writes go through this seam; which Backend serves them
is a configuration choice.
_Avoid_: datastore, database, table.

## Backend

A database that can serve Entity Storage. Today: RethinkDB and Postgres.

## Backend Routing

The per-Entity choice of which Backend serves reads and writes while the system migrates
from RethinkDB to Postgres. An Entity is served by RethinkDB, by Postgres, or by both for
the duration of its migration. Changing an Entity's Backend Routing is a configuration
change, not a code change.
