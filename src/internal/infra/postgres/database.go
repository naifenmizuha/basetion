package postgres

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed fixtures/*.sql
var fixtureFiles embed.FS

var temporaryDatabasePrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type TemporaryOption func(*temporaryOptions)

type temporaryOptions struct{ loadDevelopmentFixtures bool }

func WithDevelopmentFixtures() TemporaryOption {
	return func(options *temporaryOptions) { options.loadDevelopmentFixtures = true }
}

// ManagedStore owns a Store and, for temporary mode, the database that backs it.
type ManagedStore struct {
	Store        *Store
	admin        *pgxpool.Pool
	databaseName string
	temporary    bool

	mu       sync.Mutex
	closed   bool
	closeErr error
}

func OpenFixed(ctx context.Context, databaseURL string) (*ManagedStore, error) {
	store, err := Open(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	return &ManagedStore{Store: store}, nil
}

func OpenTemporary(ctx context.Context, adminURL, prefix string, options ...TemporaryOption) (*ManagedStore, error) {
	if adminURL == "" {
		return nil, errors.New("temporary database admin URL is required")
	}
	name, err := temporaryDatabaseName(prefix)
	if err != nil {
		return nil, err
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return nil, fmt.Errorf("configure temporary database admin pool: %w", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		return nil, fmt.Errorf("connect temporary database admin pool: %w", err)
	}
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		admin.Close()
		return nil, fmt.Errorf("create temporary database %q: %w", name, err)
	}

	managed := &ManagedStore{admin: admin, databaseName: name, temporary: true}
	poolConfig, err := pgxpool.ParseConfig(adminURL)
	if err == nil {
		poolConfig.ConnConfig.Database = name
		managed.Store, err = openWithConfig(ctx, poolConfig)
	}
	if err != nil {
		cleanupErr := cleanupTemporary(managed)
		return nil, errors.Join(fmt.Errorf("open temporary database %q: %w", name, err), cleanupErr)
	}

	settings := temporaryOptions{}
	for _, option := range options {
		option(&settings)
	}
	if settings.loadDevelopmentFixtures {
		if err := managed.Store.LoadDevelopmentFixtures(ctx); err != nil {
			cleanupErr := cleanupTemporary(managed)
			return nil, errors.Join(err, cleanupErr)
		}
	}
	return managed, nil
}

func cleanupTemporary(managed *ManagedStore) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return managed.Close(ctx)
}

func openWithConfig(ctx context.Context, config *pgxpool.Config) (*Store, error) {
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("configure PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	store := &Store{pool: pool}
	if err := store.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return store, nil
}

func temporaryDatabaseName(prefix string) (string, error) {
	if !temporaryDatabasePrefixPattern.MatchString(prefix) {
		return "", errors.New("temporary database prefix must start with a lowercase letter, contain only lowercase letters, digits, or underscores, and be at most 40 characters")
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate temporary database name: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(suffix[:]), nil
}

func (m *ManagedStore) DatabaseName() string { return m.databaseName }

func (m *ManagedStore) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.closeErr
	}
	m.closed = true
	if m.Store != nil {
		m.Store.Close()
	}
	if !m.temporary {
		return nil
	}
	if m.admin == nil {
		return errors.New("temporary database admin pool is unavailable")
	}
	defer m.admin.Close()
	if _, err := m.admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND pid<>pg_backend_pid()`, m.databaseName); err != nil {
		m.closeErr = fmt.Errorf("terminate temporary database %q connections: %w", m.databaseName, err)
		return m.closeErr
	}
	quoted := pgx.Identifier{m.databaseName}.Sanitize()
	if _, err := m.admin.Exec(ctx, "DROP DATABASE "+quoted); err != nil {
		m.closeErr = fmt.Errorf("drop temporary database %q: %w", m.databaseName, err)
		return m.closeErr
	}
	return nil
}

func (s *Store) LoadDevelopmentFixtures(ctx context.Context) error {
	entries, err := fs.ReadDir(fixtureFiles, "fixtures")
	if err != nil {
		return fmt.Errorf("read development fixtures: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		source, err := fixtureFiles.ReadFile("fixtures/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read development fixture %s: %w", entry.Name(), err)
		}
		if _, err := s.pool.Exec(ctx, string(source), pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("load development fixture %s: %w", entry.Name(), err)
		}
	}
	return nil
}
