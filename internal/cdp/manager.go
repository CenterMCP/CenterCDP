package cdp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const DefaultDatabaseID = "default"

type CustomerDatabase struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type managedDB struct {
	db *sql.DB
}

type Manager struct {
	registry *sql.DB
	dataDir  string
	logger   *zap.SugaredLogger
	mu       sync.Mutex
	open     map[string]*managedDB
}

func NewManager(registryPath, dataDir, defaultDatabasePath string, logger *zap.SugaredLogger) (*Manager, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	registry, err := openSQLite(registryPath, false)
	if err != nil {
		return nil, fmt.Errorf("open database registry: %w", err)
	}
	if _, err := registry.Exec(registrySchema); err != nil {
		registry.Close()
		return nil, fmt.Errorf("initialize database registry: %w", err)
	}

	m := &Manager{registry: registry, dataDir: dataDir, logger: logger, open: make(map[string]*managedDB)}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = registry.Exec(`INSERT INTO customer_databases
		(id,name,slug,database_path,schema_version,status,created_at,updated_at)
		VALUES(?,?,?,?,1,'active',?,?) ON CONFLICT(id) DO NOTHING`,
		DefaultDatabaseID, "Default", "default", defaultDatabasePath, now, now)
	if err != nil {
		m.Close()
		return nil, fmt.Errorf("register default database: %w", err)
	}
	if _, err := m.Get(context.Background(), DefaultDatabaseID); err != nil {
		m.Close()
		return nil, err
	}
	return m, nil
}

func openSQLite(path string, readOnly bool) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := &url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	q.Set("_foreign_keys", "on")
	if readOnly {
		q.Set("mode", "ro")
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (m *Manager) Create(ctx context.Context, name string) (*CustomerDatabase, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("database name is required")
	}
	id := "db_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	slug := slugify(name) + "-" + id[len(id)-8:]
	path := filepath.Join(m.dataDir, id+".db")
	db, err := openSQLite(path, false)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, databaseSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize customer database: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := m.registry.ExecContext(ctx, `INSERT INTO customer_databases
		(id,name,slug,database_path,schema_version,status,created_at,updated_at)
		VALUES(?,?,?,?,1,'active',?,?)`, id, name, slug, path, now, now); err != nil {
		db.Close()
		return nil, fmt.Errorf("register customer database: %w", err)
	}
	m.mu.Lock()
	m.open[id] = &managedDB{db: db}
	m.mu.Unlock()
	m.logger.Infof("Created customer database: %s", id)
	return &CustomerDatabase{ID: id, Name: name, Slug: slug, SchemaVersion: 1, Status: "active", CreatedAt: now}, nil
}

func (m *Manager) List(ctx context.Context) ([]CustomerDatabase, error) {
	rows, err := m.registry.QueryContext(ctx, `SELECT id,name,slug,schema_version,status,created_at FROM customer_databases ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CustomerDatabase
	for rows.Next() {
		var item CustomerDatabase
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.SchemaVersion, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (m *Manager) Get(ctx context.Context, id string) (*sql.DB, error) {
	m.mu.Lock()
	if item := m.open[id]; item != nil {
		m.mu.Unlock()
		return item.db, nil
	}
	m.mu.Unlock()

	var path, status string
	if err := m.registry.QueryRowContext(ctx, `SELECT database_path,status FROM customer_databases WHERE id=?`, id).Scan(&path, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("customer database %q not found", id)
		}
		return nil, err
	}
	if status != "active" {
		return nil, fmt.Errorf("customer database %q is not active", id)
	}
	db, err := openSQLite(path, false)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, databaseSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate customer database: %w", err)
	}
	m.mu.Lock()
	if existing := m.open[id]; existing != nil {
		m.mu.Unlock()
		db.Close()
		return existing.db, nil
	}
	m.open[id] = &managedDB{db: db}
	m.mu.Unlock()
	m.logger.Infof("Opened customer database: %s", id)
	return db, nil
}

func (m *Manager) ReadOnly(ctx context.Context, id string) (*sql.DB, error) {
	var path, status string
	if err := m.registry.QueryRowContext(ctx, `SELECT database_path,status FROM customer_databases WHERE id=?`, id).Scan(&path, &status); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, fmt.Errorf("customer database %q is not active", id)
	}
	return openSQLite(path, true)
}

func (m *Manager) IDs(ctx context.Context) ([]string, error) {
	items, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.Status == "active" {
			ids = append(ids, item.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var first error
	for _, item := range m.open {
		if err := item.db.Close(); err != nil && first == nil {
			first = err
		}
	}
	m.open = make(map[string]*managedDB)
	if m.registry != nil {
		if err := m.registry.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	value = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if value == "" {
		return "database"
	}
	return value
}
