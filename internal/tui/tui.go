// Package tui presents the conversational reader in a full-screen terminal UI.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/xjz6626/fanqie-tui/internal/agent"
)

var (
	accent       = lipgloss.Color("#F36D4A")
	accentSoft   = lipgloss.Color("#FFB39D")
	textColor    = lipgloss.Color("#E8E8E8")
	mutedColor   = lipgloss.Color("#888888")
	borderColor  = lipgloss.Color("#454545")
	errorColor   = lipgloss.Color("#FF6B6B")
	selectedBack = lipgloss.Color("#2B211F")
)

type message struct {
	role  string
	reply agent.Reply
}

type turnResult struct {
	replies []agent.Reply
	err     error
	book    string
	index   int
	total   int
}

// Model is the Bubble Tea application model.
type Model struct {
	agent         *agent.State
	input         textarea.Model
	viewport      viewport.Model
	spinner       spinner.Model
	messages      []message
	width         int
	height        int
	ready         bool
	busy          bool
	status        string
	requestCancel context.CancelFunc
	timeout       time.Duration
	initialPrompt string
	bookTitle     string
	chapterIndex  int
	chapterTotal  int
}

// New creates a conversation UI for a local reader agent.
func New(readerAgent *agent.State, timeout time.Duration, initialPrompt string) Model {
	input := textarea.New()
	input.Placeholder = "输入书名，或说“搜索三体”…"
	input.Prompt = "› "
	input.CharLimit = 500
	input.MaxHeight = 5
	input.ShowLineNumbers = false
	input.SetHeight(1)
	input.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accent).Bold(true)
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(textColor)
	input.BlurredStyle = input.FocusedStyle
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j"))
	_ = input.Focus()

	loading := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	loading.Style = lipgloss.NewStyle().Foreground(accent)

	return Model{
		agent:         readerAgent,
		input:         input,
		spinner:       loading,
		status:        "就绪",
		timeout:       timeout,
		initialPrompt: strings.TrimSpace(initialPrompt),
		messages: []message{{role: "assistant", reply: agent.Reply{
			Kind:  agent.KindText,
			Title: "你好，我是番茄阅读助手",
			Text:  "告诉我想看的书，我会搜索公开书籍、打开目录并陪你连续阅读。\n\n试试输入：搜索 三体",
		}}},
	}
}

// Init focuses the composer and optionally dispatches an initial instruction.
func (model Model) Init() tea.Cmd {
	commands := []tea.Cmd{textarea.Blink, model.spinner.Tick}
	if model.initialPrompt != "" {
		commands = append(commands, func() tea.Msg { return initialPromptMsg(model.initialPrompt) })
	}
	return tea.Batch(commands...)
}

type initialPromptMsg string

// Update processes terminal and completed tool messages.
func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		model.resize(typed.Width, typed.Height)
		return model, nil
	case initialPromptMsg:
		return model.submit(string(typed))
	case turnResult:
		model.busy = false
		model.requestCancel = nil
		model.status = "就绪"
		model.bookTitle = typed.book
		model.chapterIndex = typed.index
		model.chapterTotal = typed.total
		if typed.err != nil {
			model.messages = append(model.messages, message{role: "error", reply: agent.Reply{
				Kind: agent.KindText, Title: "请求失败", Text: typed.err.Error(),
			}})
		} else {
			for _, reply := range typed.replies {
				switch reply.Kind {
				case agent.KindQuit:
					return model, tea.Quit
				case agent.KindClear:
					model.messages = nil
				default:
					if reply.Kind == agent.KindChapter {
						model.foldPreviousChapters()
					}
					model.messages = append(model.messages, message{role: "assistant", reply: reply})
				}
			}
		}
		model.syncConversation(true)
		return model, model.input.Focus()
	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(typed)
		return model, command
	case tea.KeyMsg:
		switch typed.String() {
		case "ctrl+c":
			if model.busy && model.requestCancel != nil {
				if model.status == "正在取消…" {
					return model, tea.Quit
				}
				model.requestCancel()
				model.status = "正在取消…"
				return model, nil
			}
			return model, tea.Quit
		case "esc":
			if model.busy && model.requestCancel != nil {
				model.requestCancel()
				model.status = "正在取消…"
			}
			return model, nil
		case "enter":
			if !model.busy && strings.TrimSpace(model.input.Value()) != "" {
				return model.submit(model.input.Value())
			}
			return model, nil
		case "pgup", "pgdown":
			var command tea.Cmd
			model.viewport, command = model.viewport.Update(typed)
			return model, command
		}
	}

	var commands []tea.Cmd
	if !model.busy {
		var inputCommand tea.Cmd
		model.input, inputCommand = model.input.Update(msg)
		commands = append(commands, inputCommand)
	}
	var viewportCommand tea.Cmd
	model.viewport, viewportCommand = model.viewport.Update(msg)
	commands = append(commands, viewportCommand)
	return model, tea.Batch(commands...)
}

func (model *Model) foldPreviousChapters() {
	for index := range model.messages {
		chapter := model.messages[index].reply.Chapter
		if model.messages[index].reply.Kind == agent.KindChapter && chapter != nil && chapter.Content != "" {
			chapter.Content = "（上一章正文已折叠，可输入对应章节号重新打开。）"
		}
	}
}

func (model Model) submit(value string) (tea.Model, tea.Cmd) {
	instruction := strings.TrimSpace(value)
	if instruction == "" || model.busy {
		return model, nil
	}
	model.input.Reset()
	model.input.Blur()
	model.messages = append(model.messages, message{role: "user", reply: agent.Reply{Kind: agent.KindText, Text: instruction}})
	model.busy = true
	model.status = toolStatus(instruction)
	model.syncConversation(true)

	ctx, cancel := context.WithTimeout(context.Background(), model.timeout)
	model.requestCancel = cancel
	command := func() tea.Msg {
		defer cancel()
		replies, err := model.agent.Handle(ctx, instruction)
		book, index, total := model.agent.BookContext()
		return turnResult{replies: replies, err: err, book: book, index: index, total: total}
	}
	return model, tea.Batch(command, model.spinner.Tick)
}

func toolStatus(instruction string) string {
	switch {
	case strings.Contains(instruction, "搜索") || strings.Contains(instruction, "搜"):
		return "正在搜索公开书库"
	case strings.Contains(instruction, "目录") || strings.Contains(instruction, "打开"):
		return "正在读取书籍信息"
	case strings.Contains(instruction, "章") || strings.Contains(instruction, "继续"):
		return "正在获取并解码章节"
	default:
		return "正在处理"
	}
}

func (model *Model) resize(width, height int) {
	model.width = width
	model.height = height
	contentWidth := max(20, width-4)
	model.input.SetWidth(max(10, contentWidth-4))
	inputHeight := max(3, model.input.Height()+2)
	viewportHeight := max(3, height-inputHeight-5)
	if !model.ready {
		model.viewport = viewport.New(contentWidth, viewportHeight)
		model.viewport.MouseWheelEnabled = true
		model.viewport.MouseWheelDelta = 3
		model.ready = true
	} else {
		model.viewport.Width = contentWidth
		model.viewport.Height = viewportHeight
	}
	model.syncConversation(false)
}

func (model *Model) syncConversation(bottom bool) {
	if !model.ready {
		return
	}
	blocks := make([]string, 0, len(model.messages)+1)
	for _, item := range model.messages {
		blocks = append(blocks, model.renderMessage(item))
	}
	if model.busy {
		blocks = append(blocks, lipgloss.NewStyle().Foreground(mutedColor).Render(model.spinner.View()+" "+model.status))
	}
	model.viewport.SetContent(strings.Join(blocks, "\n\n"))
	if bottom {
		latestHeight := 0
		if len(blocks) > 0 {
			latestHeight = lipgloss.Height(blocks[len(blocks)-1])
		}
		startOfLatest := model.viewport.TotalLineCount() - latestHeight - 1
		maxOffset := max(0, model.viewport.TotalLineCount()-model.viewport.Height)
		model.viewport.SetYOffset(min(max(0, startOfLatest), maxOffset))
	}
}

func (model Model) renderMessage(item message) string {
	contentWidth := max(20, model.viewport.Width-2)
	bodyWidth := max(16, contentWidth-4)
	if item.role == "user" {
		label := lipgloss.NewStyle().Foreground(accentSoft).Bold(true).Render("› You")
		body := lipgloss.NewStyle().Foreground(textColor).Width(bodyWidth).PaddingLeft(2).Render(item.reply.Text)
		return label + "\n" + body
	}
	labelColor := accent
	label := "● fanqie"
	if item.role == "error" {
		labelColor = errorColor
		label = "● error"
	}
	header := lipgloss.NewStyle().Foreground(labelColor).Bold(true).Render(label)
	body := renderReply(item.reply, bodyWidth)
	return header + "\n" + lipgloss.NewStyle().PaddingLeft(2).Width(bodyWidth).Render(body)
}

func renderReply(reply agent.Reply, width int) string {
	title := ""
	if reply.Title != "" {
		title = lipgloss.NewStyle().Foreground(textColor).Bold(true).Render(reply.Title) + "\n"
	}
	muted := lipgloss.NewStyle().Foreground(mutedColor)
	regular := lipgloss.NewStyle().Foreground(textColor)

	switch reply.Kind {
	case agent.KindSearch:
		lines := []string{title}
		books := reply.Books
		if len(books) > 5 {
			books = books[:5]
		}
		for index, book := range books {
			meta := strings.Join(nonEmpty(book.Author, book.Category, book.Status, score(book.Score)), " · ")
			lines = append(lines, fmt.Sprintf("%s  %s", lipgloss.NewStyle().Foreground(accent).Bold(true).Render(fmt.Sprintf("%2d", index+1)), regular.Render(book.Title)))
			if meta != "" {
				lines = append(lines, "    "+muted.Render(meta))
			}
		}
		if len(reply.Books) > len(books) {
			lines = append(lines, "    "+muted.Render(fmt.Sprintf("…另有 %d 项结果", len(reply.Books)-len(books))))
		}
		if reply.Text != "" {
			lines = append(lines, "", muted.Render(reply.Text))
		}
		return strings.Join(lines, "\n")
	case agent.KindBook:
		if reply.Book == nil {
			return title
		}
		book := reply.Book
		metadata := strings.Join(nonEmpty(book.Author, book.Category, book.Status, compactNumber(book.WordCount)+"字", count(book.ChapterCount, "章")), " · ")
		text := title + regular.Render(book.Title)
		if metadata != "" {
			text += "\n" + muted.Render(metadata)
		}
		if book.Abstract != "" {
			text += "\n\n" + regular.Render(book.Abstract)
		}
		return text
	case agent.KindCatalog:
		lines := []string{title}
		for _, chapter := range reply.Chapters {
			lock := ""
			if chapter.Locked || chapter.NeedPay {
				lock = muted.Render("  [锁定]")
			}
			lines = append(lines, fmt.Sprintf("%4d  %s%s", chapter.Order, chapter.Title, lock))
		}
		if reply.Text != "" {
			lines = append(lines, "", muted.Render(reply.Text))
		}
		return strings.Join(lines, "\n")
	case agent.KindChapter:
		if reply.Chapter == nil {
			return title
		}
		chapter := reply.Chapter
		chapterTitle := chapter.Title
		if chapter.Order > 0 && !strings.HasPrefix(chapterTitle, "第") {
			chapterTitle = fmt.Sprintf("第 %d 章  %s", chapter.Order, chapterTitle)
		}
		return lipgloss.NewStyle().Foreground(textColor).Bold(true).Render(chapterTitle) + "\n\n" + regular.Width(width).Render(chapter.Content) + "\n\n" + muted.Render("输入“下一章”继续阅读")
	default:
		return title + regular.Width(width).Render(reply.Text)
	}
}

// View lays out the message stream, composer and status line.
func (model Model) View() string {
	if !model.ready {
		return "正在启动番茄阅读助手…"
	}
	contentWidth := max(20, model.width-4)
	brand := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("● fanqie")
	mode := lipgloss.NewStyle().Foreground(mutedColor).Render("local reading agent  ·  public web")
	header := lipgloss.NewStyle().Width(contentWidth).Render(brand + "  " + mode)

	composer := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(contentWidth - 2).
		Render(model.input.View())

	left := model.contextLabel()
	right := "Enter 发送  ·  Ctrl+J 换行  ·  PgUp/PgDn 滚动  ·  Ctrl+C 退出"
	space := contentWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		right = "Enter 发送 · Ctrl+C 退出"
		space = max(1, contentWidth-lipgloss.Width(left)-lipgloss.Width(right))
	}
	footer := lipgloss.NewStyle().Foreground(mutedColor).Width(contentWidth).Render(left + strings.Repeat(" ", space) + right)

	return lipgloss.NewStyle().Margin(1, 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, "", model.viewport.View(), composer, footer),
	)
}

func (model Model) contextLabel() string {
	if model.bookTitle == "" {
		return "未选择书籍"
	}
	label := model.bookTitle
	if model.chapterIndex > 0 && model.chapterTotal > 0 {
		label += fmt.Sprintf(" · %d/%d", model.chapterIndex, model.chapterTotal)
	}
	return label
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && value != "0字" && value != "0章" {
			result = append(result, value)
		}
	}
	return result
}

func score(value float64) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%g 分", value)
}

func count(value int, suffix string) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d%s", value, suffix)
}

func compactNumber(value int) string {
	switch {
	case value >= 100_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(value)/100_000_000), ".0") + "亿"
	case value >= 10_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(value)/10_000), ".0") + "万"
	default:
		return fmt.Sprintf("%d", value)
	}
}
