package kg

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/sergio-bershadsky/secondbrain-db/internal/semantic"
)

// Clock can be overridden by callers to make timestamps deterministic in tests.
// Default: time.Now.
var Clock = time.Now

// Logger is the slog handler used for non-fatal warnings (e.g. embedding
// failures during crawl). Default: slog.Default().
var Logger = slog.Default()

// DB wraps a SQLite database for the knowledge graph and semantic search.
type DB struct {
	db       *sql.DB
	embedder semantic.Embedder // nil = graph-only mode (no semantic search)
	dim      int
	dbPath   string
}

// Open opens or creates the SQLite knowledge graph database.
// Pass embedder=nil for graph-only mode (no semantic search).
func Open(dbPath string, embedder semantic.Embedder) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating KG directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening KG database: %w", err)
	}

	dim := 0
	if embedder != nil {
		dim = embedder.Dim()
	}

	kgdb := &DB{
		db:       db,
		embedder: embedder,
		dim:      dim,
		dbPath:   dbPath,
	}

	if err := kgdb.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating KG database: %w", err)
	}

	return kgdb, nil
}

// OpenMemory opens an in-memory SQLite database (for tests).
func OpenMemory(embedder semantic.Embedder) (*DB, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}

	dim := 0
	if embedder != nil {
		dim = embedder.Dim()
	}

	kgdb := &DB{db: db, embedder: embedder, dim: dim, dbPath: ":memory:"}
	if err := kgdb.Migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return kgdb, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Migrate creates tables if they don't exist.
func (d *DB) Migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			id          TEXT PRIMARY KEY,
			entity      TEXT NOT NULL,
			title       TEXT,
			file        TEXT,
			content_sha TEXT,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS edges (
			source_id     TEXT NOT NULL,
			source_entity TEXT NOT NULL,
			target_id     TEXT NOT NULL,
			target_entity TEXT NOT NULL,
			edge_type     TEXT NOT NULL,
			context       TEXT,
			UNIQUE(source_id, target_id, edge_type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_type   ON edges(edge_type)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			doc_id      TEXT NOT NULL,
			entity      TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			text        TEXT NOT NULL,
			content_sha TEXT NOT NULL,
			model_id    TEXT NOT NULL,
			embedding   BLOB,
			UNIQUE(doc_id, chunk_index, model_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_doc ON chunks(doc_id)`,
		`CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		)`,
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, m := range migrations {
		if _, err := tx.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %s: %w", m[:50], err)
		}
	}

	return tx.Commit()
}

// SetMeta stores a metadata key-value pair.
func (d *DB) SetMeta(key, value string) error {
	_, err := d.db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?`,
		key, value, value,
	)
	return err
}

// GetMeta retrieves a metadata value. Returns "" if not found.
func (d *DB) GetMeta(key string) (string, error) {
	var value string
	err := d.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// Stats returns summary statistics about the knowledge graph.
type Stats struct {
	Nodes     int    `json:"nodes"`
	Edges     int    `json:"edges"`
	Chunks    int    `json:"chunks"`
	ModelID   string `json:"model_id"`
	DBSize    int64  `json:"db_size_bytes"`
	UpdatedAt string `json:"updated_at"`
}

func (d *DB) Stats() (*Stats, error) {
	s := &Stats{}

	d.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&s.Nodes)
	d.db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&s.Edges)
	d.db.QueryRow(`SELECT COUNT(*) FROM chunks`).Scan(&s.Chunks)

	modelID, _ := d.GetMeta("model_id")
	s.ModelID = modelID
	updatedAt, _ := d.GetMeta("last_build_at")
	s.UpdatedAt = updatedAt

	if d.dbPath != ":memory:" {
		if fi, err := os.Stat(d.dbPath); err == nil {
			s.DBSize = fi.Size()
		}
	}

	return s, nil
}

// Drop removes all data from the knowledge graph.
func (d *DB) Drop() error {
	tables := []string{"nodes", "edges", "chunks", "meta"}
	for _, t := range tables {
		if _, err := d.db.Exec("DELETE FROM " + t); err != nil {
			return fmt.Errorf("dropping %s: %w", t, err)
		}
	}
	return nil
}

func nowRFC3339() string {
	return Clock().UTC().Format(time.RFC3339)
}
