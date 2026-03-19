package ui

import (
	"fmt"
	"os"
	"strings"

	"atlas.ed/internal/editor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#D4AF37")).
			Padding(0, 1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555")).
			Padding(0, 1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D4AF37")).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA"))

	matchCountStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#D4AF37")).
			Padding(0, 1).
			Bold(true)
			
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#555555")).
			Padding(0, 1)

	modeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#D4AF37")).
			Bold(true).
			Padding(0, 1)

	confirmStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#D4AF37")).
			Padding(1, 2)
)

type Mode int

const (
	ModeEdit Mode = iota
	ModeSearchInput
	ModeSearchNav
	ModeQuitConfirm
)

type Model struct {
	filename        string
	initialContent  string
	textarea        textarea.Model
	searchInput     textinput.Model
	viewport        viewport.Model
	mode            Mode
	showLineNumbers bool
	modified        bool
	
	// Undo/Redo
	undoStack []string
	redoStack []string
	
	// Search
	searchQuery string
	matches     []int
	matchIndex  int
	
	// Cache
	highlightedContent   string
	lastHighlightedValue string

	width  int
	height int
}

func NewModel(filename string, content string) Model {
	ta := textarea.New()
	ta.Placeholder = "Start typing..."
	ta.SetValue(content)
	ta.SetCursor(0) // Start at the top
	ta.Focus()
	ta.ShowLineNumbers = true
	ta.LineNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4AF37"))

	si := textinput.New()
	si.Placeholder = "Search query..."
	si.Prompt = " / "

	vp := viewport.New(80, 20)

	return Model{
		filename:        filename,
		initialContent:  content,
		textarea:        ta,
		searchInput:     si,
		viewport:        vp,
		mode:            ModeEdit,
		showLineNumbers: true,
		matchIndex:      -1,
	}
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global Quit/Interrupt handling
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+q" {
			if m.modified {
				m.mode = ModeQuitConfirm
				return m, nil
			}
			return m, tea.Quit
		}

		// Handle Quit Confirmation
		if m.mode == ModeQuitConfirm {
			switch msg.String() {
			case "y", "Y":
				m.saveFile()
				return m, tea.Quit
			case "n", "N":
				return m, tea.Quit
			case "esc":
				m.mode = ModeEdit
				return m, nil
			}
			return m, nil
		}

		// Handle Search Input Mode
		if m.mode == ModeSearchInput {
			switch msg.String() {
			case "enter":
				m.searchQuery = m.searchInput.Value()
				if m.searchQuery == "" {
					m.mode = ModeEdit
					return m, nil
				}
				m.performSearch()
				if len(m.matches) > 0 {
					m.mode = ModeSearchNav
					m.updateViewport()
				} else {
					m.mode = ModeEdit
				}
				return m, nil
			case "esc":
				m.mode = ModeEdit
				return m, nil
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}

		// Handle Search Navigation Mode
		if m.mode == ModeSearchNav {
			switch msg.String() {
			case "enter":
				m.mode = ModeEdit
				return m, nil
			case "n":
				m.findNext()
				m.updateViewport()
				return m, nil
			case "p", "N":
				m.findPrev()
				m.updateViewport()
				return m, nil
			case "esc", "q":
				m.mode = ModeEdit
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		// Handle Edit Mode
		switch msg.String() {
		case "ctrl+z":
			m.undo()
			return m, nil
		case "ctrl+y":
			m.redo()
			return m, nil
		case "ctrl+s":
			m.saveFile()
			return m, nil
		case "ctrl+f":
			m.mode = ModeSearchInput
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			return m, textinput.Blink
		case "ctrl+l":
			m.showLineNumbers = !m.showLineNumbers
			m.textarea.ShowLineNumbers = m.showLineNumbers
		case "pgup":
			for i := 0; i < m.textarea.Height(); i++ {
				m.textarea.CursorUp()
			}
		case "pgdown":
			for i := 0; i < m.textarea.Height(); i++ {
				m.textarea.CursorDown()
			}
		case "home":
			m.textarea.CursorStart()
		case "end":
			m.textarea.CursorEnd()
		}

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		headerHeight := lipgloss.Height(m.headerView())
		footerHeight := lipgloss.Height(m.footerView())
		
		m.textarea.SetWidth(msg.Width)
		m.textarea.SetHeight(msg.Height - headerHeight - footerHeight)
		
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
	}

	if m.mode == ModeEdit {
		var taCmd tea.Cmd
		
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			s := kmsg.String()
			// Keys that definitely don't change content (navigation, toggle, find, quit)
			isNav := s == "up" || s == "down" || s == "left" || s == "right" ||
				s == "pgup" || s == "pgdown" || s == "home" || s == "end" ||
				s == "ctrl+s" || s == "ctrl+f" || s == "ctrl+l" || s == "ctrl+q" || s == "ctrl+c" ||
				s == "ctrl+z" || s == "ctrl+y" || s == "esc"

			if !isNav {
				prevVal := m.textarea.Value()
				m.textarea, taCmd = m.textarea.Update(msg)
				newVal := m.textarea.Value()
				if newVal != prevVal {
					m.modified = true
					m.undoStack = append(m.undoStack, prevVal)
					m.redoStack = nil
					if len(m.undoStack) > 100 {
						m.undoStack = m.undoStack[1:]
					}
				}
			} else {
				m.textarea, taCmd = m.textarea.Update(msg)
			}
		} else {
			// Not a KeyMsg (e.g., Blink, WindowSizeMsg)
			m.textarea, taCmd = m.textarea.Update(msg)
		}
		cmds = append(cmds, taCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) updateViewport() {
	content := m.textarea.Value()
	if content != m.lastHighlightedValue {
		highlighted, _ := editor.Highlight(content, m.filename)
		m.highlightedContent = highlighted
		m.lastHighlightedValue = content
	}
	
	final := editor.HighlightSearch(m.highlightedContent, m.searchQuery, m.matchIndex)
	
	if m.showLineNumbers {
		var sb strings.Builder
		lines := strings.Split(final, "\n")
		width := len(fmt.Sprintf("%d", len(lines)))
		for i, line := range lines {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#D4AF37")).Render(fmt.Sprintf("%*d ", width, i+1)))
			sb.WriteString(line)
			if i < len(lines)-1 {
				sb.WriteByte('\n')
			}
		}
		final = sb.String()
	}
	m.viewport.SetContent(final)
	
	if len(m.matches) > 0 && m.matchIndex >= 0 {
		offset := m.matches[m.matchIndex]
		lineNum := strings.Count(content[:offset], "\n")
		m.viewport.SetYOffset(lineNum)
	}
}

func (m *Model) undo() {
	if len(m.undoStack) == 0 {
		return
	}

	// Current state to redo stack
	m.redoStack = append(m.redoStack, m.textarea.Value())

	// Pop from undo stack
	prev := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]

	m.textarea.SetValue(prev)
	m.modified = true // Usually undoing still counts as modified if it's different from initial
	if m.textarea.Value() == m.initialContent {
		m.modified = false
	}
}

func (m *Model) redo() {
	if len(m.redoStack) == 0 {
		return
	}

	// Current state to undo stack
	m.undoStack = append(m.undoStack, m.textarea.Value())

	// Pop from redo stack
	next := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]

	m.textarea.SetValue(next)
	m.modified = true
	if m.textarea.Value() == m.initialContent {
		m.modified = false
	}
}

func (m *Model) saveFile() {
	_ = os.WriteFile(m.filename, []byte(m.textarea.Value()), 0644)
	m.modified = false
}

func (m *Model) performSearch() {
	m.matches = nil
	content := strings.ToLower(m.textarea.Value())
	query := strings.ToLower(m.searchQuery)
	start := 0
	for {
		idx := strings.Index(content[start:], query)
		if idx == -1 { break }
		m.matches = append(m.matches, start+idx)
		start += idx + len(query)
	}
	if len(m.matches) > 0 { 
		m.matchIndex = 0 
		m.jumpToMatch()
	} else { 
		m.matchIndex = -1 
	}
}

func (m *Model) findNext() {
	if len(m.matches) == 0 { return }
	m.matchIndex = (m.matchIndex + 1) % len(m.matches)
	m.jumpToMatch()
}

func (m *Model) findPrev() {
	if len(m.matches) == 0 { return }
	m.matchIndex = (m.matchIndex - 1 + len(m.matches)) % len(m.matches)
	m.jumpToMatch()
}

func (m *Model) jumpToMatch() {
	if m.matchIndex < 0 { return }
	
	offset := m.matches[m.matchIndex]
	plain := m.textarea.Value()

	targetLine := strings.Count(plain[:offset], "\n")
	
	lastNewLine := strings.LastIndex(plain[:offset], "\n")
	targetCol := 0
	if lastNewLine == -1 {
		targetCol = len([]rune(plain[:offset]))
	} else {
		targetCol = len([]rune(plain[lastNewLine+1 : offset]))
	}

	for m.textarea.Line() < targetLine {
		m.textarea.CursorDown()
	}
	for m.textarea.Line() > targetLine {
		m.textarea.CursorUp()
	}
	
	m.textarea.SetCursor(targetCol)
}

func (m Model) View() string {
	var body string
	if m.mode == ModeSearchNav {
		body = m.viewport.View()
	} else {
		body = m.textarea.View()
	}
	
	view := fmt.Sprintf("%s\n%s\n%s", m.headerView(), body, m.footerView())
	
	if m.mode == ModeQuitConfirm {
		dialog := confirmStyle.Render("Unsaved changes! Save before quitting?\n\n(y)es / (n)o / (esc) cancel")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}
	
	return view
}

func (m Model) headerView() string {
	var currentMode string
	switch m.mode {
	case ModeSearchNav:
		currentMode = " SEARCH MODE "
	case ModeSearchInput:
		currentMode = " INPUT QUERY "
	default:
		currentMode = " EDIT MODE "
	}
	
	modChar := ""
	if m.modified { modChar = "*" }
	
	title := titleStyle.Render("ATLAS ED")
	mLabel := modeStyle.Render(currentMode)
	status := statusStyle.Render(" " + m.filename + modChar + " ")
	
	line := strings.Repeat("─", max(0, m.width-lipgloss.Width(title)-lipgloss.Width(mLabel)-lipgloss.Width(status)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, mLabel, status, infoStyle.Render(line))
}

func (m Model) footerView() string {
	if m.mode == ModeSearchInput {
		return m.searchInput.View()
	}

	var help string
	if m.mode == ModeSearchNav {
		help = lipgloss.JoinHorizontal(lipgloss.Top,
			helpKeyStyle.Render(" enter "), helpDescStyle.Render("go to match "),
			helpKeyStyle.Render(" n/p "), helpDescStyle.Render("next/prev "),
			helpKeyStyle.Render(" q/esc "), helpDescStyle.Render("stop search "),
			helpKeyStyle.Render(" ^Q "), helpDescStyle.Render("quit "),
		)
	} else {
		help = lipgloss.JoinHorizontal(lipgloss.Top,
			helpKeyStyle.Render(" ^S "), helpDescStyle.Render("save "),
			helpKeyStyle.Render(" ^Z "), helpDescStyle.Render("undo "),
			helpKeyStyle.Render(" ^Y "), helpDescStyle.Render("redo "),
			helpKeyStyle.Render(" ^F "), helpDescStyle.Render("find "),
			helpKeyStyle.Render(" ^L "), helpDescStyle.Render("lines "),
			helpKeyStyle.Render(" ^Q "), helpDescStyle.Render("quit "),
		)
	}
	
	matchInfo := ""
	if len(m.matches) > 0 {
		matchInfo = matchCountStyle.Render(fmt.Sprintf(" MATCH %d/%d ", m.matchIndex+1, len(m.matches)))
	}

	gap := max(0, m.width-lipgloss.Width(help)-lipgloss.Width(matchInfo)-2)
	line := strings.Repeat(" ", gap)
	
	return lipgloss.JoinHorizontal(lipgloss.Center, help, line, matchInfo)
}

func max(a, b int) int {
	if a > b { return a }
	return b
}
