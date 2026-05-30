package session

import (
	"context"
	"encoding/json"
	"fmt"
)

// StoreSummary stores a message summary.
func (s *SQLiteStorage) StoreSummary(ctx context.Context, summary *Summary) error {
	// Convert message IDs to JSON.
	messageIDsJSON, err := json.Marshal(summary.OriginalMessageIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal message IDs: %w", err)
	}

	query := `INSERT INTO message_summaries (id, session_id, provider, original_message_ids, message_range, summary_content, token_count, compacted_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = s.db.ExecContext(ctx, query,
		summary.ID,
		summary.SessionID,
		summary.Provider,
		string(messageIDsJSON),
		summary.MessageRange,
		summary.SummaryContent,
		summary.TokenCount,
		summary.CompactedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert summary: %w", err)
	}

	return nil
}

// GetSummaries retrieves all summaries for a session.
func (s *SQLiteStorage) GetSummaries(ctx context.Context, sessionID string) ([]*Summary, error) {
	query := `SELECT id, session_id, provider, original_message_ids, message_range, summary_content, token_count, compacted_at
	          FROM message_summaries
	          WHERE session_id = ?
	          ORDER BY compacted_at ASC`

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*Summary

	for rows.Next() {
		var summary Summary
		var messageIDsJSON string

		err := rows.Scan(
			&summary.ID,
			&summary.SessionID,
			&summary.Provider,
			&messageIDsJSON,
			&summary.MessageRange,
			&summary.SummaryContent,
			&summary.TokenCount,
			&summary.CompactedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}

		// Unmarshal message IDs.
		if err := json.Unmarshal([]byte(messageIDsJSON), &summary.OriginalMessageIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message IDs: %w", err)
		}

		summaries = append(summaries, &summary)
	}

	// Check for errors from iterating over rows.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating summaries: %w", err)
	}

	return summaries, nil
}

// ArchiveMessages marks messages as archived.
func (s *SQLiteStorage) ArchiveMessages(ctx context.Context, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}

	// Build query with placeholders.
	query := `UPDATE messages SET archived = 1 WHERE id IN (`
	params := make([]interface{}, len(messageIDs))
	for i, id := range messageIDs {
		if i > 0 {
			query += ", "
		}
		query += "?"
		params[i] = id
	}
	query += ")"

	_, err := s.db.ExecContext(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("failed to archive messages: %w", err)
	}

	return nil
}

// GetActiveMessages retrieves non-archived messages for a session.
func (s *SQLiteStorage) GetActiveMessages(ctx context.Context, sessionID string, limit int) ([]*Message, error) {
	query := `SELECT id, session_id, role, content, created_at, COALESCE(archived, 0) as archived
	          FROM messages
	          WHERE session_id = ? AND COALESCE(archived, 0) = 0
	          ORDER BY created_at ASC`

	if limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active messages: %w", err)
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
			&message.Archived,
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
