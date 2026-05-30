package step

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var tableTestInitOnce sync.Once

// initTableTestIO initializes the I/O context for table tests.
func initTableTestIO(t *testing.T) {
	t.Helper()
	tableTestInitOnce.Do(func() {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	})
}

// TableHandler registration and basic validation are tested in output_handlers_test.go.
// This file tests helper methods.

func TestTableHandler_DetermineColumns(t *testing.T) {
	handler, ok := Get("table")
	require.True(t, ok)
	tableHandler := handler.(*TableHandler)

	t.Run("uses explicit columns", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Columns: []string{"name", "value", "status"},
			Data: []map[string]any{
				{"name": "a", "value": 1, "status": "ok"},
			},
		}
		columns := tableHandler.determineColumns(step)
		assert.Equal(t, []string{"name", "value", "status"}, columns)
	})

	t.Run("derives columns from data", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Data: []map[string]any{
				{"name": "a", "value": 1},
			},
		}
		columns := tableHandler.determineColumns(step)
		// Order may vary since it's derived from map keys.
		assert.Len(t, columns, 2)
		assert.Contains(t, columns, "name")
		assert.Contains(t, columns, "value")
	})

	t.Run("returns nil for empty data and no columns", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Data: []map[string]any{},
		}
		columns := tableHandler.determineColumns(step)
		assert.Nil(t, columns)
	})

	t.Run("returns nil for nil data and no columns", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
		}
		columns := tableHandler.determineColumns(step)
		assert.Nil(t, columns)
	})
}

func TestTableHandler_BuildHeader(t *testing.T) {
	handler, ok := Get("table")
	require.True(t, ok)
	tableHandler := handler.(*TableHandler)

	t.Run("builds header with nil styles", func(t *testing.T) {
		columns := []string{"Name", "Value"}
		header := tableHandler.buildHeader(columns, nil)
		assert.Equal(t, []string{"Name", "Value"}, header)
	})

	t.Run("builds empty header", func(t *testing.T) {
		columns := []string{}
		header := tableHandler.buildHeader(columns, nil)
		assert.Empty(t, header)
	})
}

func TestTableHandler_BuildRows(t *testing.T) {
	handler, ok := Get("table")
	require.True(t, ok)
	tableHandler := handler.(*TableHandler)

	t.Run("builds rows from data", func(t *testing.T) {
		data := []map[string]any{
			{"name": "alice", "age": 30},
			{"name": "bob", "age": 25},
		}
		columns := []string{"name", "age"}
		rows := tableHandler.buildRows(data, columns)
		assert.Len(t, rows, 2)
		assert.Equal(t, "alice\t30", rows[0])
		assert.Equal(t, "bob\t25", rows[1])
	})

	t.Run("handles missing columns in data", func(t *testing.T) {
		data := []map[string]any{
			{"name": "alice"},
		}
		columns := []string{"name", "age"}
		rows := tableHandler.buildRows(data, columns)
		assert.Len(t, rows, 1)
		assert.Equal(t, "alice\t", rows[0])
	})

	t.Run("handles empty data", func(t *testing.T) {
		data := []map[string]any{}
		columns := []string{"name", "age"}
		rows := tableHandler.buildRows(data, columns)
		assert.Empty(t, rows)
	})

	t.Run("handles various value types", func(t *testing.T) {
		data := []map[string]any{
			{"str": "hello", "int": 42, "float": 3.14, "bool": true},
		}
		columns := []string{"str", "int", "float", "bool"}
		rows := tableHandler.buildRows(data, columns)
		assert.Len(t, rows, 1)
		assert.Contains(t, rows[0], "hello")
		assert.Contains(t, rows[0], "42")
		assert.Contains(t, rows[0], "3.14")
		assert.Contains(t, rows[0], "true")
	})
}

func TestTableHandler_AddTitle(t *testing.T) {
	handler, ok := Get("table")
	require.True(t, ok)
	tableHandler := handler.(*TableHandler)

	t.Run("adds title when present", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Title: "My Table",
		}
		vars := NewVariables()
		output, err := tableHandler.addTitle("content", step, vars, nil)
		require.NoError(t, err)
		assert.Contains(t, output, "My Table")
		assert.Contains(t, output, "content")
	})

	t.Run("returns original when no title", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
		}
		vars := NewVariables()
		output, err := tableHandler.addTitle("content", step, vars, nil)
		require.NoError(t, err)
		assert.Equal(t, "content", output)
	})

	t.Run("resolves template in title", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Title: "Table for {{ .steps.env.value }}",
		}
		vars := NewVariables()
		vars.Set("env", NewStepResult("production"))
		output, err := tableHandler.addTitle("content", step, vars, nil)
		require.NoError(t, err)
		assert.Contains(t, output, "Table for production")
	})

	t.Run("returns error for invalid title template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Title: "{{ .steps.invalid.value",
		}
		vars := NewVariables()
		_, err := tableHandler.addTitle("content", step, vars, nil)
		assert.Error(t, err)
	})
}

func TestTableHandler_Execute(t *testing.T) {
	initTableTestIO(t)

	handler, ok := Get("table")
	require.True(t, ok)

	t.Run("executes with content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "table",
			Content: "Name\tValue\nAlice\t100",
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Alice")
	})

	t.Run("executes with content and template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "table",
			Content: "Name\tValue\n{{ .steps.name.value }}\t100",
		}
		vars := NewVariables()
		vars.Set("name", NewStepResult("Bob"))
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Bob")
	})

	t.Run("executes with data", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Type: "table",
			Data: []map[string]any{
				{"name": "Charlie", "value": 200},
			},
			Columns: []string{"name", "value"},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "Charlie")
		assert.Contains(t, result.Value, "200")
	})

	t.Run("executes with data and title", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "table",
			Title: "User Data",
			Data: []map[string]any{
				{"name": "Dana"},
			},
			Columns: []string{"name"},
		}
		vars := NewVariables()
		ctx := context.Background()

		result, err := handler.Execute(ctx, step, vars)
		require.NoError(t, err)
		assert.Contains(t, result.Value, "User Data")
		assert.Contains(t, result.Value, "Dana")
	})

	t.Run("returns error for invalid content template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Type:    "table",
			Content: "{{ .invalid",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("returns error for invalid title template with data", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:  "test",
			Type:  "table",
			Title: "{{ .invalid",
			Data: []map[string]any{
				{"name": "Eve"},
			},
			Columns: []string{"name"},
		}
		vars := NewVariables()
		ctx := context.Background()

		_, err := handler.Execute(ctx, step, vars)
		assert.Error(t, err)
	})
}
