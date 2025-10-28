package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// DefaultDirPerms is the default permissions for creating storage directories.
	defaultDirPerms = 0o755
)

// SQLiteStorage implements Storage using SQLite.
type SQLiteStorage struct {
	db   *sql.DB
	path string
}

// NewSQLiteStorage creates a new SQLite storage backend.
func NewSQLiteStorage(storagePath string) (*SQLiteStorage, error) {
	// Ensure directory exists.
	dir := filepath.Dir(storagePath)
	if err := os.MkdirAll(dir, defaultDirPerms); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Open database.
	db, err := sql.Open("sqlite", storagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection.
	db.SetMaxOpenConns(1) // SQLite works best with single connection

	// Enable performance optimizations.
	pragmas := []string{
		"PRAGMA foreign_keys = ON",     // Required for cascade deletes
		"PRAGMA journal_mode = WAL",    // Write-Ahead Logging: faster, non-blocking writes
		"PRAGMA synchronous = NORMAL",  // Reduce fsyncs while maintaining crash safety
		"PRAGMA busy_timeout = 5000",   // Wait up to 5s if database is locked
		"PRAGMA cache_size = -64000",   // 64MB cache (negative = KB, positive = pages)
		"PRAGMA temp_store = MEMORY",   // Store temp tables in memory
		"PRAGMA mmap_size = 268435456", // 256MB memory-mapped I/O for faster reads
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%w %q: %w", errUtils.ErrAISQLitePragmaFailed, pragma, err)
		}
	}

	storage := &SQLiteStorage{
		db:   db,
		path: storagePath,
	}

	// Run migrations.
	if err := storage.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return storage, nil
}

// Migrate creates or updates the database schema.
func (s *SQLiteStorage) Migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			project_path TEXT NOT NULL,
			model TEXT NOT NULL,
			provider TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			metadata TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_project_path ON sessions(project_path)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_name ON sessions(project_path, name)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at)`,

		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(session_id, created_at)`,

		`CREATE TABLE IF NOT EXISTS session_context (
			session_id TEXT NOT NULL,
			context_type TEXT NOT NULL,
			context_key TEXT NOT NULL,
			context_value TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_context_session_id ON session_context(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_context_type ON session_context(session_id, context_type)`,
	}

	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Add agent column to sessions table (migration for existing databases).
	// Check if column exists first since SQLite doesn't support IF NOT EXISTS in ALTER TABLE.
	var columnExists bool
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='agent'`).Scan(&columnExists)
	if err != nil {
		return fmt.Errorf("failed to check for agent column: %w", err)
	}

	if !columnExists {
		// Use empty string as default for backward compatibility.
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN agent TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("failed to add agent column: %w", err)
		}
	}

	return nil
}

// CreateSession creates a new session.
func (s *SQLiteStorage) CreateSession(ctx context.Context, session *Session) error {
	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `INSERT INTO sessions (id, name, project_path, model, provider, agent, created_at, updated_at, metadata)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		session.ID,
		session.Name,
		session.ProjectPath,
		session.Model,
		session.Provider,
		session.Agent,
		session.CreatedAt,
		session.UpdatedAt,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID.
func (s *SQLiteStorage) GetSession(ctx context.Context, id string) (*Session, error) {
	query := `SELECT id, name, project_path, model, provider, agent, created_at, updated_at, metadata
	          FROM sessions WHERE id = ?`

	var session Session
	var metadataJSON string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&session.ID,
		&session.Name,
		&session.ProjectPath,
		&session.Model,
		&session.Provider,
		&session.Agent,
		&session.CreatedAt,
		&session.UpdatedAt,
		&metadataJSON,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errUtils.ErrAISessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Unmarshal metadata.
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &session, nil
}

// GetSessionByName retrieves a session by name and project path.
func (s *SQLiteStorage) GetSessionByName(ctx context.Context, projectPath, name string) (*Session, error) {
	query := `SELECT id, name, project_path, model, provider, agent, created_at, updated_at, metadata
	          FROM sessions WHERE project_path = ? AND name = ?
	          ORDER BY updated_at DESC LIMIT 1`

	var session Session
	var metadataJSON string

	err := s.db.QueryRowContext(ctx, query, projectPath, name).Scan(
		&session.ID,
		&session.Name,
		&session.ProjectPath,
		&session.Model,
		&session.Provider,
		&session.Agent,
		&session.CreatedAt,
		&session.UpdatedAt,
		&metadataJSON,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errUtils.ErrAISessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Unmarshal metadata.
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &session, nil
}

// ListSessions returns all sessions for a project.
func (s *SQLiteStorage) ListSessions(ctx context.Context, projectPath string, limit int) ([]*Session, error) {
	query := `SELECT s.id, s.name, s.project_path, s.model, s.provider, s.agent, s.created_at, s.updated_at, s.metadata, COALESCE(COUNT(m.id), 0) as message_count
	          FROM sessions s
	          LEFT JOIN messages m ON s.id = m.session_id
	          WHERE s.project_path = ?
	          GROUP BY s.id
	          ORDER BY s.updated_at DESC
	          LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, projectPath, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session

	for rows.Next() {
		var session Session
		var metadataJSON string

		err := rows.Scan(
			&session.ID,
			&session.Name,
			&session.ProjectPath,
			&session.Model,
			&session.Provider,
			&session.Agent,
			&session.CreatedAt,
			&session.UpdatedAt,
			&metadataJSON,
			&session.MessageCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		// Unmarshal metadata.
		if metadataJSON != "" {
			if err := json.Unmarshal([]byte(metadataJSON), &session.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		sessions = append(sessions, &session)
	}

	// Check for errors from iterating over rows.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	return sessions, nil
}

// UpdateSession updates an existing session.
func (s *SQLiteStorage) UpdateSession(ctx context.Context, session *Session) error {
	metadataJSON, err := json.Marshal(session.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `UPDATE sessions SET name = ?, model = ?, provider = ?, agent = ?, updated_at = ?, metadata = ?
	          WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query,
		session.Name,
		session.Model,
		session.Provider,
		session.Agent,
		session.UpdatedAt,
		string(metadataJSON),
		session.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errUtils.ErrAISessionNotFound
	}

	return nil
}

// DeleteSession deletes a session.
func (s *SQLiteStorage) DeleteSession(ctx context.Context, id string) error {
	query := `DELETE FROM sessions WHERE id = ?`

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return errUtils.ErrAISessionNotFound
	}

	return nil
}

// DeleteOldSessions deletes sessions older than the specified time.
func (s *SQLiteStorage) DeleteOldSessions(ctx context.Context, olderThan time.Time) (int, error) {
	query := `DELETE FROM sessions WHERE updated_at < ?`

	result, err := s.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old sessions: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rows), nil
}

// AddMessage adds a message to a session.
func (s *SQLiteStorage) AddMessage(ctx context.Context, message *Message) error {
	query := `INSERT INTO messages (session_id, role, content, created_at)
	          VALUES (?, ?, ?, ?)`

	result, err := s.db.ExecContext(ctx, query,
		message.SessionID,
		message.Role,
		message.Content,
		message.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Get the inserted ID.
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	message.ID = id

	return nil
}

// GetMessages retrieves messages for a session.
func (s *SQLiteStorage) GetMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	query := `SELECT id, session_id, role, content, created_at
	          FROM messages WHERE session_id = ?
	          ORDER BY created_at ASC`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message

	for rows.Next() {
		var message Message

		err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&message.Role,
			&message.Content,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		messages = append(messages, &message)
	}

	// Check for errors from iterating over rows.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return messages, nil
}

// GetMessageCount returns the number of messages in a session.
func (s *SQLiteStorage) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	query := `SELECT COUNT(*) FROM messages WHERE session_id = ?`

	var count int
	err := s.db.QueryRowContext(ctx, query, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}

// AddContext adds a context item to a session.
func (s *SQLiteStorage) AddContext(ctx context.Context, item *ContextItem) error {
	query := `INSERT INTO session_context (session_id, context_type, context_key, context_value)
	          VALUES (?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		item.SessionID,
		item.ContextType,
		item.ContextKey,
		item.ContextValue,
	)
	if err != nil {
		return fmt.Errorf("failed to insert context: %w", err)
	}

	return nil
}

// GetContext retrieves all context items for a session.
func (s *SQLiteStorage) GetContext(ctx context.Context, sessionID string) ([]*ContextItem, error) {
	query := `SELECT session_id, context_type, context_key, context_value
	          FROM session_context WHERE session_id = ?`

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query context: %w", err)
	}
	defer rows.Close()

	var items []*ContextItem

	for rows.Next() {
		var item ContextItem

		err := rows.Scan(
			&item.SessionID,
			&item.ContextType,
			&item.ContextKey,
			&item.ContextValue,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan context: %w", err)
		}

		items = append(items, &item)
	}

	// Check for errors from iterating over rows.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating context: %w", err)
	}

	return items, nil
}

// DeleteContext deletes all context items for a session.
func (s *SQLiteStorage) DeleteContext(ctx context.Context, sessionID string) error {
	query := `DELETE FROM session_context WHERE session_id = ?`

	_, err := s.db.ExecContext(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete context: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
