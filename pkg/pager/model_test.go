package pager

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestModel_Init(t *testing.T) {
	m := model{}
	cmd := m.Init()
	assert.Nil(t, cmd, "Init should return nil")
}

func TestModel_Update(t *testing.T) {
	t.Run("KeyMsg_Quit", func(t *testing.T) {
		m := model{}
		keys := []string{"ctrl+c", "q", "esc"}
		for _, key := range keys {
			t.Run(key, func(t *testing.T) {
				msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
				updatedModel, cmd := m.Update(msg)
				gotModel := updatedModel.(*model)
				assert.Equal(t, m, *gotModel, "Model should not change")
				assert.Equal(t, reflect.ValueOf(tea.Quit).Pointer(), reflect.ValueOf(cmd).Pointer(), "Command should be tea.Quit")
			})
		}
	})

	t.Run("WindowSizeMsg_NotReady", func(t *testing.T) {
		m := model{
			content: "test content",
			title:   "Test Title",
			ready:   false,
		}
		msg := tea.WindowSizeMsg{Width: 80, Height: 24}
		updatedModel, cmd := m.Update(msg)

		assert.True(t, updatedModel.(*model).ready, "Model should be ready")
		assert.Equal(t, 80, updatedModel.(*model).viewport.Width, "Viewport width should be set")
		assert.Equal(t, 18, updatedModel.(*model).viewport.Height, "Viewport height should account for header and footer")
		assert.Equal(t, 3, updatedModel.(*model).viewport.YPosition, "YPosition should be header height")
		assert.Contains(t, updatedModel.(*model).viewport.View(), "test content", "Content should be set")
		assert.Nil(t, cmd, "No additional command expected")
	})

	t.Run("WindowSizeMsg_Ready", func(t *testing.T) {
		m := model{
			ready:    true,
			viewport: viewport.New(100, 20),
		}
		msg := tea.WindowSizeMsg{Width: 120, Height: 30}
		updatedModel, cmd := m.Update(msg)

		assert.True(t, updatedModel.(*model).ready, "Model should remain ready")
		assert.Equal(t, 120, updatedModel.(*model).viewport.Width, "Viewport width should be updated")
		assert.Equal(t, 24, updatedModel.(*model).viewport.Height, "Viewport height should be updated")
		assert.Nil(t, cmd, "No additional command expected")
	})

	t.Run("ViewportUpdate", func(t *testing.T) {
		vp := viewport.New(80, 20)
		m := model{
			ready:    true,
			viewport: vp,
		}
		msg := tea.KeyMsg{Type: tea.KeyDown}
		updatedModel, cmd := m.Update(msg)

		assert.True(t, updatedModel.(*model).ready, "Model should remain ready")
		assert.NotNil(t, updatedModel.(*model).viewport, "Viewport should still exist")
		if cmd != nil {
			assert.NotNil(t, cmd, "Viewport may return a command")
		}
	})
}

func TestModel_View(t *testing.T) {
	t.Run("NotReady", func(t *testing.T) {
		m := model{ready: false}
		output := m.View()
		assert.Equal(t, "\n  Initializing...", output, "View should return initializing message")
	})

	t.Run("Ready", func(t *testing.T) {
		vp := viewport.New(80, 20)
		vp.SetContent("test content")
		m := model{
			ready:    true,
			title:    "Test Title",
			viewport: vp,
		}
		output := m.View()

		assert.Contains(t, output, "Test Title", "Output should contain title")
		assert.Contains(t, output, "test content", "Output should contain viewport content")
		assert.Contains(t, output, "%", "Output should contain scroll percentage")
	})
}

func TestModel_headerView(t *testing.T) {
	t.Run("NormalWidth", func(t *testing.T) {
		m := model{
			title:    "Test",
			viewport: viewport.New(20, 10),
		}
		header := m.headerView()

		expectedTitle := titleStyle.Render("Test")
		lineLength := 20 - lipgloss.Width(expectedTitle)
		assert.Contains(t, header, "Test", "Header should contain title")
		assert.Contains(t, header, strings.Repeat("─", lineLength), "Header should contain line")
	})
}

func TestModel_footerView(t *testing.T) {
	t.Run("NormalWidth", func(t *testing.T) {
		vp := viewport.New(20, 10)
		// Simulate scrolling by setting content and updating YOffset
		vp.SetContent(strings.Repeat("line\n", 20)) // Enough lines to scroll
		vp.YOffset = 5                              // Scroll halfway
		m := model{
			viewport: vp,
		}
		footer := m.footerView()

		// ScrollPercent() should reflect ~50% (exact value depends on content height)
		expectedInfo := infoStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))
		lineLength := 20 - lipgloss.Width(expectedInfo)
		assert.Contains(t, footer, fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100), "Footer should contain scroll percentage")
		assert.Contains(t, footer, strings.Repeat("─", lineLength), "Footer should contain line")
	})

	t.Run("ZeroLineLength", func(t *testing.T) {
		vp := viewport.New(5, 10)
		m := model{
			viewport: vp,
		}
		footer := m.footerView()

		assert.Contains(t, footer, "100%", "Footer should contain scroll percentage")
	})
}

func TestMax(t *testing.T) {
	t.Run("A_Greater", func(t *testing.T) {
		result := max(5, 3)
		assert.Equal(t, 5, result, "max(5, 3) should return 5")
	})

	t.Run("B_Greater", func(t *testing.T) {
		result := max(3, 5)
		assert.Equal(t, 5, result, "max(3, 5) should return 3")
	})

	t.Run("Equal", func(t *testing.T) {
		result := max(4, 4)
		assert.Equal(t, 4, result, "max(4, 4) should return 4")
	})
}

func TestStyles(t *testing.T) {
	t.Run("TitleStyle", func(t *testing.T) {
		style := titleStyle.Render("Test")
		assert.Contains(t, style, "Test", "Title style should render content")
		assert.True(t, len(style) > len("Test"), "Style should add border/padding")
	})

	t.Run("InfoStyle", func(t *testing.T) {
		style := infoStyle.Render("50%")
		assert.Contains(t, style, "50%", "Info style should render content")
		assert.True(t, len(style) > len("50%"), "Style should add border/padding")
	})
}
