// Package agent implements the local, conversational reading workflow.
package agent

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
)

// Kind categorizes an agent event for the terminal presentation layer.
type Kind int

const (
	KindText Kind = iota
	KindSearch
	KindBook
	KindCatalog
	KindChapter
	KindStatus
	KindClear
	KindQuit
)

// Reply is one or more structured messages from a local agent turn.
type Reply struct {
	Kind     Kind
	Title    string
	Text     string
	Books    []fanqie.Book
	Book     *fanqie.Book
	Chapters []fanqie.Chapter
	Chapter  *fanqie.ChapterContent
}

// State keeps the context needed for follow-up instructions.
type State struct {
	Provider fanqie.Provider
	Book     *fanqie.Book
	Books    []fanqie.Book
	Chapters []fanqie.Chapter
	Index    int
}

var (
	searchPattern  = regexp.MustCompile(`^(?:/search\s+|搜索|搜一下|找书|帮我找)(.+)$`)
	openPattern    = regexp.MustCompile(`^(?:/open\s+|打开|选择|看)(?:第)?\s*(\d+)\s*(?:本|个)?$`)
	readPattern    = regexp.MustCompile(`^(?:/read\s+|(?:读|阅读|跳到|跳转到)\s*(?:第)?\s*|从第\s*)(\d+)\s*(?:章)?(?:开始)?(?:读|阅读)?$`)
	chapterPattern = regexp.MustCompile(`^第\s*(\d+)\s*章$`)
)

// New creates a stateful local agent.
func New(provider fanqie.Provider) *State {
	return &State{Provider: provider, Index: -1}
}

// Handle interprets a natural-language instruction and performs its tools.
func (state *State) Handle(ctx context.Context, input string) ([]Reply, error) {
	command := strings.TrimSpace(input)
	if command == "" {
		return nil, nil
	}
	lower := strings.ToLower(command)

	switch lower {
	case "/quit", "/exit", "退出", "再见":
		return []Reply{{Kind: KindQuit}}, nil
	case "/clear", "清空对话":
		return []Reply{{Kind: KindClear}}, nil
	case "/help", "帮助", "怎么用", "你能做什么":
		return []Reply{{Kind: KindText, Title: "使用帮助", Text: helpText()}}, nil
	case "/status", "状态", "现在读到哪":
		return []Reply{{Kind: KindStatus, Title: "当前会话", Text: state.status()}}, nil
	case "/catalog", "目录", "查看目录", "看看目录":
		return state.catalog(ctx)
	case "/next", "下一章", "继续", "继续读", "接着读":
		return state.move(ctx, 1)
	case "/prev", "/previous", "上一章", "返回上一章":
		return state.move(ctx, -1)
	}

	if matches := searchPattern.FindStringSubmatch(command); len(matches) == 2 {
		return state.search(ctx, strings.TrimSpace(matches[1]))
	}
	if matches := openPattern.FindStringSubmatch(command); len(matches) == 2 {
		return state.open(ctx, matches[1])
	}
	if matches := readPattern.FindStringSubmatch(command); len(matches) == 2 {
		return state.readOrder(ctx, matches[1])
	}
	if matches := chapterPattern.FindStringSubmatch(command); len(matches) == 2 {
		return state.readOrder(ctx, matches[1])
	}
	if strings.HasPrefix(lower, "/search ") {
		return state.search(ctx, strings.TrimSpace(command[len("/search "):]))
	}
	if state.Book == nil {
		return state.search(ctx, command)
	}
	return []Reply{{
		Kind:  KindText,
		Title: "我没完全理解",
		Text:  "可以直接说“下一章”“查看目录”“从第 20 章开始读”，或者输入 /help 查看全部用法。",
	}}, nil
}

// BookContext returns the current book and reading position for presentation.
func (state *State) BookContext() (title string, position int, total int) {
	if state.Book == nil {
		return "", 0, 0
	}
	return state.Book.Title, state.Index + 1, len(state.Chapters)
}

func (state *State) search(ctx context.Context, query string) ([]Reply, error) {
	page, err := state.Provider.Search(ctx, query, 0)
	if err != nil {
		return nil, err
	}
	state.Books = page.Books
	if len(page.Books) == 0 {
		return []Reply{{Kind: KindText, Title: "搜索完成", Text: "没有找到匹配的书籍，可以换个书名或作者试试。"}}, nil
	}
	return []Reply{{
		Kind:  KindSearch,
		Title: fmt.Sprintf("找到 %d 本相关书籍", len(page.Books)),
		Text:  "输入“打开 1”选择一本书。",
		Books: page.Books,
	}}, nil
}

func (state *State) open(ctx context.Context, rawIndex string) ([]Reply, error) {
	position, _ := strconv.Atoi(rawIndex)
	if position < 1 || position > len(state.Books) {
		if len(state.Books) == 0 {
			return []Reply{{Kind: KindText, Title: "还没有搜索结果", Text: "先输入“搜索 书名”，再输入“打开 1”。"}}, nil
		}
		return []Reply{{Kind: KindText, Title: "序号超出范围", Text: fmt.Sprintf("请输入 1 到 %d 之间的书籍序号。", len(state.Books))}}, nil
	}
	book, err := state.Provider.GetBook(ctx, state.Books[position-1].ID)
	if err != nil {
		return nil, err
	}
	chapters, err := state.Provider.GetDirectory(ctx, book.ID)
	if err != nil {
		return nil, err
	}
	state.Book = &book
	state.Chapters = chapters
	state.Index = -1
	return []Reply{
		{Kind: KindBook, Title: "已打开书籍", Book: &book},
		{Kind: KindText, Text: fmt.Sprintf("目录共 %d 章。可以说“从第 1 章开始读”或“查看目录”。", len(chapters))},
	}, nil
}

func (state *State) catalog(ctx context.Context) ([]Reply, error) {
	if state.Book == nil {
		return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先搜索并打开一本书。"}}, nil
	}
	if len(state.Chapters) == 0 {
		chapters, err := state.Provider.GetDirectory(ctx, state.Book.ID)
		if err != nil {
			return nil, err
		}
		state.Chapters = chapters
	}
	start := 0
	if state.Index >= 0 {
		start = state.Index - 5
		if start < 0 {
			start = 0
		}
	}
	end := start + 20
	if end > len(state.Chapters) {
		end = len(state.Chapters)
	}
	return []Reply{{
		Kind:     KindCatalog,
		Title:    fmt.Sprintf("%s · 目录 %d–%d / %d", state.Book.Title, start+1, end, len(state.Chapters)),
		Text:     "输入“第 12 章”即可跳转。",
		Chapters: state.Chapters[start:end],
	}}, nil
}

func (state *State) readOrder(ctx context.Context, rawOrder string) ([]Reply, error) {
	if state.Book == nil {
		return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先搜索并打开一本书。"}}, nil
	}
	order, _ := strconv.Atoi(rawOrder)
	index := -1
	for position, chapter := range state.Chapters {
		if chapter.Order == order {
			index = position
			break
		}
	}
	if index < 0 && order >= 1 && order <= len(state.Chapters) {
		index = order - 1
	}
	if index < 0 {
		return []Reply{{Kind: KindText, Title: "章节不存在", Text: fmt.Sprintf("当前目录共 %d 章。", len(state.Chapters))}}, nil
	}
	return state.readIndex(ctx, index)
}

func (state *State) move(ctx context.Context, delta int) ([]Reply, error) {
	if state.Book == nil {
		return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先搜索并打开一本书。"}}, nil
	}
	index := state.Index + delta
	if state.Index < 0 && delta > 0 {
		index = 0
	}
	if index < 0 || index >= len(state.Chapters) {
		return []Reply{{Kind: KindText, Title: "无法继续", Text: "已经到达目录边界。"}}, nil
	}
	return state.readIndex(ctx, index)
}

func (state *State) readIndex(ctx context.Context, index int) ([]Reply, error) {
	entry := state.Chapters[index]
	if entry.Locked || entry.NeedPay {
		state.Index = index
		return []Reply{{Kind: KindText, Title: entry.Title, Text: "该章节已锁定或需要付费，请使用官方客户端阅读。"}}, nil
	}
	chapter, err := state.Provider.GetChapter(ctx, entry.ID)
	if err != nil {
		return nil, err
	}
	state.Index = index
	return []Reply{{Kind: KindChapter, Title: chapter.Title, Chapter: &chapter}}, nil
}

func (state *State) status() string {
	if state.Book == nil {
		return "尚未打开书籍。输入书名即可搜索。"
	}
	location := "尚未开始阅读"
	if state.Index >= 0 && state.Index < len(state.Chapters) {
		location = state.Chapters[state.Index].Title
	}
	return fmt.Sprintf("书籍：%s\n作者：%s\n进度：%s\n目录：%d 章", state.Book.Title, state.Book.Author, location, len(state.Chapters))
}

func helpText() string {
	return "直接输入自然语言即可：\n\n" +
		"  搜索 三体\n  打开 1\n  从第 3 章开始读\n  下一章 / 上一章\n  查看目录\n  现在读到哪\n\n" +
		"斜杠命令：\n" +
		"  /search  /open  /read  /catalog\n" +
		"  /next  /prev  /status  /clear  /quit"
}
