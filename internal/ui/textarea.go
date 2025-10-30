package ui

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type Prompt struct {
	textarea.Model
	termW int
	minH  int
	maxH  int
	lastH int
}

func NewPrompt() Prompt {
	ti := textarea.New()

	// ti.Placeholder = "Type your message..."
	ti.Focus()
	ti.Prompt = userStyle.Render("\u2588 ")
	ti.Placeholder = "Type your message..."
	ti.CharLimit = 0
	// ti.Width = 80
	ti.SetWidth(80)
	// ti.PromptStyle.Background(lipgloss.Color("235"))

	ti.ShowLineNumbers = false
	ti.SetHeight(1)

	ti.FocusedStyle.Base.Background(lipgloss.Color("235"))

	// ti.PromptStyle
	ti.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	ti.FocusedStyle.Base.Border(lipgloss.BlockBorder(), true, false).BorderForeground(lipgloss.Color("34")).BorderBackground(lipgloss.Color("235"))
	ti.BlurredStyle.Base.Border(lipgloss.BlockBorder(), true, false).BorderForeground(lipgloss.Color("34")).BorderBackground(lipgloss.Color("235"))

	// ti.BlurredStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))

	return Prompt{
		Model: ti,
		minH:  1,
		maxH:  20,
		lastH: -1,
	}
}

func (p Prompt) View() string {
	return p.Model.View()
}

func (p Prompt) Update(msg tea.Msg) (Prompt, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.termW = msg.Width
		// set textarea width if desired (subtract padding)
		p.Model.SetWidth(p.termW)
	case tea.KeyMsg:
		// let textarea handle keys
	}

	// recompute needed height based on content and textarea width
	needed := wrapLinesCount(p.Model.Value(), p.Model.Width())
	if needed < p.minH {
		needed = p.minH
	}
	if needed > p.maxH {
		needed = p.maxH
	}
	if needed != p.lastH {
		p.Model.SetHeight(needed)
		p.lastH = needed
	}

	p.Model, cmd = p.Model.Update(msg)

	return p, cmd
}

func wrapLinesCount(s string, width int) int {
	if width <= 0 {
		return utf8.RuneCountInString(s) // fallback
	}
	lines := strings.Split(s, "\n")
	total := 0
	for _, l := range lines {
		if l == "" {
			total += 1
			continue
		}
		// compute display width of the line
		w := 0
		for _, r := range l {
			w += runewidth.RuneWidth(r)
		}
		// ceil division
		total += int(math.Ceil(float64(w) / float64(width)))
	}
	if total == 0 {
		return 1
	}
	return total + 1
}
