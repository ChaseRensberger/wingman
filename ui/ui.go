package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"wingman/agent"
	"wingman/provider"
	"wingman/session"
)

type Model struct {
	session      *session.Session
	provider     provider.Provider
	agent        *agent.Agent
	messages     []Message
	viewport     viewport.Model
	textarea     textarea.Model
	spinner      spinner.Model
	styles       Styles
	width        int
	height       int
	ready        bool
	thinking     bool
	err          error
	inputTokens  int
	outputTokens int
	quitting     bool
}

type thinkingMsg struct{}
type responseMsg struct {
	text         string
	inputTokens  int
	outputTokens int
	err          error
}

func New(s *session.Session, p provider.Provider, a *agent.Agent) Model {
	ta := textarea.New()
	ta.Placeholder = "Type your message..."
	ta.Focus()
	ta.Prompt = "│ "
	ta.CharLimit = 10000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	return Model{
		session:  s,
		provider: p,
		agent:    a,
		messages: []Message{},
		viewport: vp,
		textarea: ta,
		spinner:  sp,
		styles:   DefaultStyles(),
		width:    80,
		height:   24,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if m.thinking {
				return m, nil
			}

			userMsg := strings.TrimSpace(m.textarea.Value())
			if userMsg == "" {
				return m, nil
			}

			if userMsg == "/exit" || userMsg == "/quit" {
				m.quitting = true
				return m, tea.Quit
			}

			if userMsg == "/help" {
				m.messages = append(m.messages, Message{
					Role:    "assistant",
					Content: getHelpText(),
				})
				m.textarea.Reset()
				m.updateViewport()
				return m, nil
			}

			if userMsg == "/clear" {
				m.messages = []Message{}
				m.textarea.Reset()
				m.updateViewport()
				return m, nil
			}

			m.messages = append(m.messages, Message{
				Role:    "user",
				Content: userMsg,
			})

			m.textarea.Reset()
			m.thinking = true
			m.updateViewport()

			return m, tea.Batch(
				func() tea.Msg { return thinkingMsg{} },
				m.runAgent(userMsg),
			)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 3
		footerHeight := 6
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, msg.Height-verticalMargin)
			m.viewport.Style = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - verticalMargin
		}

		m.textarea.SetWidth(msg.Width - 4)
		m.updateViewport()

	case thinkingMsg:
		return m, m.spinner.Tick

	case responseMsg:
		m.thinking = false
		m.inputTokens += msg.inputTokens
		m.outputTokens += msg.outputTokens

		if msg.err != nil {
			m.err = msg.err
			m.messages = append(m.messages, Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Error: %v", msg.err),
			})
		} else {
			m.messages = append(m.messages, Message{
				Role:    "assistant",
				Content: msg.text,
			})
		}

		m.updateViewport()
		return m, nil

	case spinner.TickMsg:
		if m.thinking {
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if !m.ready {
		return "Initializing..."
	}

	var sb strings.Builder

	header := RenderHeader(fmt.Sprintf("Wingcode Interactive Session - %s", m.session.ID()[:8]), m.styles)
	sb.WriteString(header)
	sb.WriteString("\n\n")

	sb.WriteString(m.viewport.View())
	sb.WriteString("\n\n")

	inputBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(m.width - 4).
		Render(m.textarea.View())

	sb.WriteString(inputBox)
	sb.WriteString("\n")

	if m.thinking {
		statusText := m.spinner.View() + " Thinking..."
		sb.WriteString(m.styles.StatusBar.Render(statusText))
	} else {
		statusBar := RenderStatusBar(m.inputTokens, m.outputTokens, m.session.ID(), m.styles, m.width)
		sb.WriteString(statusBar)
	}

	return sb.String()
}

func (m *Model) updateViewport() {
	var content strings.Builder

	for _, msg := range m.messages {
		content.WriteString(RenderMessage(msg, m.styles, m.viewport.Width-4))
		content.WriteString("\n")
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

func (m Model) runAgent(prompt string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result, err := m.session.Run(ctx, prompt)
		if err != nil {
			return responseMsg{err: err}
		}

		return responseMsg{
			text:         result.Response,
			inputTokens:  result.Usage.InputTokens,
			outputTokens: result.Usage.OutputTokens,
		}
	}
}

func getHelpText() string {
	return `**Available Commands:**

- **/help** - Show this help message
- **/clear** - Clear conversation history
- **/exit** or **/quit** - Exit Wingcode
- **Ctrl+C** - Exit Wingcode

Just type your message and press Enter to chat with the agent.`
}
