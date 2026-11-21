// Package routing implements the storage seam for the gradual RethinkDB →
// Postgres migration (Option 4 in pkg/db/generic/pg/README.md).
//
// It provides a backend-agnostic Storage port with adapters for RethinkDB and
// Postgres, plus a per-entity Router that dispatches to one of them based on a
// Config. Each entity can be configured to be served by:
//
//   - rethink:  only RethinkDB
//   - postgres: only Postgres
//   - both:     written to both, read from the configured read backend
//
// The "both" mode is the transition mode: it keeps RethinkDB authoritative and
// current while Postgres is fed in parallel, so a read switch (or rollback) is
// a configuration change.
package routing

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Mode selects which backend(s) serve an entity.
type Mode string

const (
	// ModeRethink serves the entity exclusively from RethinkDB.
	ModeRethink Mode = "rethink"
	// ModePostgres serves the entity exclusively from Postgres.
	ModePostgres Mode = "postgres"
	// ModeBoth writes to both backends and reads from Config.ReadFrom. It is the
	// transition mode for a gradual cutover.
	ModeBoth Mode = "both"
)

// Valid reports whether the mode is one of the known modes.
func (m Mode) Valid() bool {
	switch m {
	case ModeRethink, ModePostgres, ModeBoth:
		return true
	default:
		return false
	}
}

// Config decides the storage mode per entity. The zero value is valid and means
// "everything stays on RethinkDB".
type Config struct {
	// Default is used for entities without an explicit entry. Empty defaults to
	// ModeRethink.
	Default Mode
	// ReadFrom is the backend reads are served from in ModeBoth. Empty defaults
	// to ModeRethink.
	ReadFrom Mode
	// Entities maps an entity name (the Go type name, e.g. "Machine") to its mode.
	Entities map[string]Mode
}

// EntityName returns the config key for an entity type T: its Go type name with
// any pointer indirection removed (e.g. "Machine").
func EntityName[T any]() string {
	t := reflect.TypeFor[T]()
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// Validate checks that all modes are known and that, in ModeBoth, ReadFrom is
// one of "rethink" or "postgres".
func (c Config) Validate() error {
	if c.Default != "" && !c.Default.Valid() {
		return fmt.Errorf("invalid default mode %q", c.Default)
	}
	if c.ReadFrom != "" && c.ReadFrom != ModeRethink && c.ReadFrom != ModePostgres {
		return fmt.Errorf("invalid read backend %q: must be %q or %q", c.ReadFrom, ModeRethink, ModePostgres)
	}

	entities := make([]string, 0, len(c.Entities))
	for entity := range c.Entities {
		entities = append(entities, entity)
	}
	sort.Strings(entities)
	for _, entity := range entities {
		if mode := c.Entities[entity]; !mode.Valid() {
			return fmt.Errorf("invalid mode %q for entity %q", mode, entity)
		}
	}

	return nil
}

// ModeFor returns the configured mode for the given entity, falling back to
// Default (or ModeRethink if unset). Entity names are matched case-insensitively
// (Parse lowercases its keys), so both "Machine" and "machine" resolve.
func (c Config) ModeFor(entity string) Mode {
	if mode, ok := c.Entities[entity]; ok {
		return mode
	}
	for name, mode := range c.Entities {
		if strings.EqualFold(name, entity) {
			return mode
		}
	}
	if c.Default != "" {
		return c.Default
	}
	return ModeRethink
}

// readBackend returns the backend reads are served from for a given mode: the
// configured ReadFrom in ModeBoth, otherwise the mode itself.
func (c Config) readBackend(mode Mode) Mode {
	if mode == ModeBoth {
		if c.ReadFrom != "" {
			return c.ReadFrom
		}
		return ModeRethink
	}
	return mode
}

// Parse parses a compact configuration string:
//
//	"default=rethink,read=rethink,machine=both,network=postgres"
//
// The keys "default" (also "mode") and "read" (also "readfrom") are reserved;
// every other key is an entity name. Whitespace around keys and values is
// trimmed; an empty string yields the zero Config.
func Parse(s string) (Config, error) {
	cfg := Config{Entities: map[string]Mode{}}

	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid config entry %q: expected key=value", part)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.ToLower(strings.TrimSpace(value))

		switch key {
		case "default", "mode":
			cfg.Default = Mode(value)
		case "read", "readfrom":
			cfg.ReadFrom = Mode(value)
		default:
			cfg.Entities[key] = Mode(value)
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
