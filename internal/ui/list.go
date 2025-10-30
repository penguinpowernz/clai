package ui

import (
	"fmt"
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type SimpleList struct {
	items    []string
	selected int
	title    string
}

func NewSimpleList(title string, items ...string) *SimpleList {
	return &SimpleList{
		items: items,
		title: title,
	}
}

func (s SimpleList) Init() tea.Cmd {
	return nil
}

func (s SimpleList) View() string {
	var b strings.Builder
	b.WriteString("\n" + s.title + ":\n\n")

	for i, option := range s.items {
		if i == s.selected {
			b.WriteString(assistantStyle.Render(fmt.Sprintf("> %s\n", option)))
			continue
		}
		b.WriteString(fmt.Sprintf("  %s\n", option))
	}

	return b.String()
}

func (s *SimpleList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if s.selected > 0 {
				s.selected--
			}
			return s, nil
		case tea.KeyDown:
			if s.selected < len(s.items)-1 {
				s.selected++
			}
			return s, nil
		case tea.KeyEnter:
			log.Println("[ui.list] got enter")
			return s, func() tea.Msg {
				log.Println("[ui.list] sending done event")
				return EventListDone{s.title, s.items[s.selected]}
			}
		}
	}
	return s, nil
}
