// Package agent implements the local, conversational reading workflow.
package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
	"github.com/xjz6626/fanqie-tui/internal/library"
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
	Provider       fanqie.Provider
	Book           *fanqie.Book
	Books          []fanqie.Book
	Chapters       []fanqie.Chapter
	Index          int
	Query          string
	Feed           fanqie.DiscoverKind
	FeedName       string
	CategoryID     string
	CategoryGender string
	CategoryName   string
	NextPage       int
	HasMore        bool
	Authors        []fanqie.Author
	Reviews        []fanqie.ReviewSummary
	Library        *library.Store
	Session        SessionActions
}

// SessionActions lets the application shell describe and remove a persisted
// browser session without coupling the conversational agent to filesystem
// paths. Empty fields keep the agent fully usable in anonymous mode.
type SessionActions struct {
	LoginGuide         string
	SessionDescription string
	Login              func(context.Context) (string, error)
	Logout             func(context.Context) error
}

var (
	searchPattern  = regexp.MustCompile(`^(?:/search\s+|搜索|搜一下|找书|帮我找)(.+)$`)
	openPattern    = regexp.MustCompile(`^(?:/open\s+|打开|选择|看)(?:第)?\s*(\d+)\s*(?:本|个)?$`)
	readPattern    = regexp.MustCompile(`^(?:/read\s+|(?:读|阅读|跳到|跳转到)\s*(?:第)?\s*|从第\s*)(\d+)\s*(?:章)?(?:开始)?(?:读|阅读)?$`)
	chapterPattern = regexp.MustCompile(`^第\s*(\d+)\s*章$`)
	authorPattern  = regexp.MustCompile(`^(?:/author\s+|作者\s*)(\d+)$`)
	commentPattern = regexp.MustCompile(`^(?:/comment\s+|/review\s+|书评\s+|评论\s+)(\S+)$`)
	commentURL     = regexp.MustCompile(`^https://fanqienovel\.com/comment/(\d+)-(\d+)(?:[?#].*)?$`)
)

// New creates a stateful local agent.
func New(provider fanqie.Provider) *State {
	return &State{Provider: provider, Index: -1}
}

// NewWithLibrary creates an agent with persistent local history, favorites,
// and reading settings.
func NewWithLibrary(provider fanqie.Provider, localLibrary *library.Store) *State {
	state := New(provider)
	state.Library = localLibrary
	return state
}

// WithSessionActions configures optional persisted-login UX and logout. The
// callback should delete the saved credential and clear it from the active
// provider. Cookie values must never be included in these display strings.
func (state *State) WithSessionActions(actions SessionActions) *State {
	state.Session = actions
	return state
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
	case "/search", "搜索", "搜一下", "找书", "帮我找":
		return []Reply{{Kind: KindText, Title: "请输入书名", Text: "例如：搜索 三体"}}, nil
	case "/open", "打开", "选择":
		return []Reply{{Kind: KindText, Title: "请输入书籍序号", Text: "例如：打开 1"}}, nil
	case "/read", "读", "阅读", "跳到", "跳转到":
		return []Reply{{Kind: KindText, Title: "请输入章节号", Text: "例如：从第 3 章开始读"}}, nil
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
	case "/more", "更多", "更多结果", "下一页":
		return state.more(ctx)
	case "/popular", "热门榜", "热门书籍":
		return state.discover(ctx, fanqie.DiscoverPopular, "热门榜")
	case "/recommend", "推荐榜", "编辑推荐":
		return state.discover(ctx, fanqie.DiscoverRecommended, "编辑推荐")
	case "/male", "男频", "男频精选":
		return state.discover(ctx, fanqie.DiscoverMale, "男频精选")
	case "/female", "女频", "女频精选":
		return state.discover(ctx, fanqie.DiscoverFemale, "女频精选")
	case "/updates", "/recent", "最近更新", "最新更新":
		return state.discover(ctx, fanqie.DiscoverRecent, "最近更新")
	case "/published", "出版榜", "出版书籍":
		return state.discover(ctx, fanqie.DiscoverPublished, "出版榜")
	case "/categories", "分类", "全部分类", "分类列表":
		return state.categories(ctx)
	case "/authors", "热门作者", "作者榜", "作家榜":
		return state.authors(ctx)
	case "/favorite", "收藏", "收藏当前", "加入收藏", "加入书架":
		return state.addFavorite(ctx)
	case "/unfavorite", "取消收藏", "移出收藏":
		return state.removeFavorite(ctx)
	case "/favorites", "收藏夹", "我的收藏", "本地书架":
		return state.favorites(ctx)
	case "/history", "历史", "历史记录", "阅读历史":
		return state.history()
	case "/resume", "继续阅读", "继续上次阅读", "恢复阅读":
		return state.resume(ctx)
	case "/account", "账号", "我的账号", "登录状态":
		return state.account(ctx)
	case "/login", "登录":
		return state.login(ctx)
	case "/logout", "退出登录", "注销登录":
		return state.logout(ctx)
	case "/cloud-history", "云端历史", "云端进度", "官网历史":
		return state.cloudHistory(ctx)
	case "/read-items", "已读章节", "官网已读":
		return state.readItems(ctx)
	case "/bookshelf", "官网书架", "云端书架":
		return state.bookshelf(ctx)
	case "/sync", "同步", "立即同步", "同步账号", "同步书架":
		return state.syncAccount(ctx)
	case "/reviews", "书评", "评论", "本书书评":
		return state.reviews(ctx, true)
	case "/review-feed", "最新书评", "书评广场":
		return state.reviews(ctx, false)
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
	if matches := authorPattern.FindStringSubmatch(command); len(matches) == 2 {
		return state.author(ctx, matches[1])
	}
	if matches := commentPattern.FindStringSubmatch(command); len(matches) == 2 {
		return state.comment(ctx, matches[1])
	}
	if strings.HasPrefix(lower, "/search ") {
		return state.search(ctx, strings.TrimSpace(command[len("/search "):]))
	}
	if strings.HasPrefix(lower, "/category ") {
		return state.category(ctx, strings.TrimSpace(command[len("/category "):]))
	}
	if strings.HasPrefix(command, "分类 ") {
		return state.category(ctx, strings.TrimSpace(command[len("分类 "):]))
	}
	if strings.HasPrefix(lower, "/author ") {
		return state.author(ctx, strings.TrimSpace(command[len("/author "):]))
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

func (state *State) login(ctx context.Context) ([]Reply, error) {
	if state.Session.Login == nil {
		return []Reply{{Kind: KindText, Title: "安全登录", Text: state.loginHelp()}}, nil
	}
	message, err := state.Session.Login(ctx)
	if err != nil {
		return []Reply{{
			Kind:  KindText,
			Title: "登录未完成",
			Text:  err.Error() + "\n\n" + state.loginHelp(),
		}}, nil
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Cookie 已通过官网登录验证并保存；以后直接启动 fanqie 即可自动登录。"
	}
	return []Reply{{Kind: KindStatus, Title: "登录成功", Text: message}}, nil
}

// BookContext returns the current book and reading position for presentation.
func (state *State) BookContext() (title string, position int, total int) {
	if state.Book == nil {
		return "", 0, 0
	}
	return state.Book.Title, state.Index + 1, len(state.Chapters)
}

func (state *State) search(ctx context.Context, query string) ([]Reply, error) {
	query = strings.TrimSpace(query)
	page, err := state.Provider.Search(ctx, query, 0)
	if err != nil {
		return nil, err
	}
	state.Books = page.Books
	state.Query = query
	state.Feed = ""
	state.FeedName = ""
	state.clearCategory()
	state.NextPage = page.NextOffset
	state.HasMore = page.HasMore
	if len(page.Books) == 0 {
		return []Reply{{Kind: KindText, Title: "搜索完成", Text: "没有找到匹配的书籍，可以换个书名或作者试试。"}}, nil
	}
	return []Reply{{
		Kind:  KindSearch,
		Title: fmt.Sprintf("找到 %d 本相关书籍", len(page.Books)),
		Text:  searchHint(page.HasMore),
		Books: page.Books,
	}}, nil
}

func (state *State) discover(ctx context.Context, kind fanqie.DiscoverKind, name string) ([]Reply, error) {
	page, err := state.Provider.Discover(ctx, kind, 0)
	if err != nil {
		return nil, err
	}
	state.Books = page.Books
	state.Query = ""
	state.Feed = kind
	state.FeedName = name
	state.clearCategory()
	state.NextPage = page.NextOffset
	state.HasMore = page.HasMore
	if len(page.Books) == 0 {
		return []Reply{{Kind: KindText, Title: name, Text: "当前榜单暂时没有可用书籍。"}}, nil
	}
	return []Reply{{
		Kind:  KindSearch,
		Title: fmt.Sprintf("%s · %d 本", name, len(page.Books)),
		Text:  searchHint(page.HasMore),
		Books: page.Books,
	}}, nil
}

func (state *State) categories(ctx context.Context) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.CategoryProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "分类榜不可用", Text: "当前数据源没有实现官网分类接口。"}}, nil
	}
	categories, err := provider.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string][]string{"male": {}, "female": {}}
	for _, category := range categories {
		groups[category.Gender] = append(groups[category.Gender], category.Name)
	}
	text := "男频：" + strings.Join(groups["male"], "、") + "\n\n女频：" + strings.Join(groups["female"], "、") +
		"\n\n输入“分类 男 西方奇幻”或“分类 女 古风世情”查看榜单；也支持 /category female 古风世情。"
	return []Reply{{Kind: KindText, Title: fmt.Sprintf("小说分类 · %d 项", len(categories)), Text: text}}, nil
}

func (state *State) category(ctx context.Context, input string) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.CategoryProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "分类榜不可用", Text: "当前数据源没有实现官网分类榜接口。"}}, nil
	}
	fields := strings.Fields(strings.TrimSpace(input))
	gender := ""
	nameParts := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.ToLower(field) {
		case "男", "男频", "male":
			gender = "male"
		case "女", "女频", "female":
			gender = "female"
		default:
			nameParts = append(nameParts, field)
		}
	}
	name := strings.Join(nameParts, " ")
	if name == "" {
		return []Reply{{Kind: KindText, Title: "请输入分类", Text: "例如：分类 男 西方奇幻；输入“分类”可查看全部分类。"}}, nil
	}
	categories, err := provider.GetCategories(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]fanqie.Category, 0, 2)
	for _, category := range categories {
		if (category.ID == name || category.Name == name) && (gender == "" || category.Gender == gender) {
			matches = append(matches, category)
		}
	}
	if len(matches) == 0 {
		return []Reply{{Kind: KindText, Title: "没有这个分类", Text: "输入“分类”查看官网当前提供的分类名称。"}}, nil
	}
	if len(matches) > 1 {
		return []Reply{{Kind: KindText, Title: "请指定男频或女频", Text: fmt.Sprintf("“%s”同时出现在男频和女频，请输入“分类 男 %s”或“分类 女 %s”。", name, name, name)}}, nil
	}
	category := matches[0]
	page, err := provider.GetCategoryRank(ctx, category.ID, category.Gender, 0)
	if err != nil {
		return nil, err
	}
	state.Books = page.Books
	state.Query = ""
	state.Feed = ""
	state.FeedName = ""
	state.CategoryID = category.ID
	state.CategoryGender = category.Gender
	state.CategoryName = genderName(category.Gender) + " · " + category.Name
	state.NextPage = page.NextOffset
	state.HasMore = page.HasMore
	if len(page.Books) == 0 {
		return []Reply{{Kind: KindText, Title: state.CategoryName, Text: "当前分类榜暂时没有可用书籍。"}}, nil
	}
	return []Reply{{Kind: KindSearch, Title: fmt.Sprintf("%s · %d 本", state.CategoryName, len(page.Books)), Text: searchHint(page.HasMore), Books: page.Books}}, nil
}

func (state *State) authors(ctx context.Context) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.AuthorProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "作者榜不可用", Text: "当前数据源没有实现官网作者接口。"}}, nil
	}
	authors, err := provider.GetTopAuthors(ctx)
	if err != nil {
		return nil, err
	}
	state.Authors = authors
	lines := make([]string, 0, len(authors)+1)
	for index, author := range authors {
		meta := strings.Join(nonEmptyAgent(author.Level, author.Introduction), " · ")
		if meta == "" {
			lines = append(lines, fmt.Sprintf("%d. %s", index+1, author.Name))
		} else {
			lines = append(lines, fmt.Sprintf("%d. %s · %s", index+1, author.Name, meta))
		}
	}
	lines = append(lines, "", "输入“作者 1”查看作者资料与作品。")
	return []Reply{{Kind: KindText, Title: fmt.Sprintf("热门作者 · %d 位", len(authors)), Text: strings.Join(lines, "\n")}}, nil
}

func (state *State) author(ctx context.Context, input string) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.AuthorProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "作者资料不可用", Text: "当前数据源没有实现官网作者接口。"}}, nil
	}
	input = strings.TrimSpace(input)
	authorID := input
	if position, err := strconv.Atoi(input); err == nil && position >= 1 && position <= len(state.Authors) {
		authorID = state.Authors[position-1].ID
	} else if len(input) <= 4 {
		return []Reply{{Kind: KindText, Title: "请先查看作者榜", Text: "输入“热门作者”，再输入“作者 1”；也可以直接使用完整作者 ID。"}}, nil
	}
	profile, books, err := provider.GetAuthor(ctx, authorID)
	if err != nil {
		return nil, err
	}
	metadata := nonEmptyAgent(
		profile.Introduction,
		labelValue("粉丝", profile.Followers),
		labelValue("创作字数", profile.WordCount),
		labelValue("创作天数", profile.CreationDays),
	)
	text := strings.Join(metadata, "\n")
	if len(books) == 0 {
		return []Reply{{Kind: KindText, Title: profile.Name, Text: firstNonEmpty(text, "官网没有返回公开作品。")}}, nil
	}
	state.setBookList(books)
	if text != "" {
		text += "\n\n"
	}
	text += "输入“打开 1”查看作品。"
	return []Reply{{Kind: KindSearch, Title: fmt.Sprintf("%s · %d 本作品", profile.Name, len(books)), Text: text, Books: books}}, nil
}

func (state *State) more(ctx context.Context) ([]Reply, error) {
	if state.Query == "" && state.Feed == "" && state.CategoryID == "" {
		return []Reply{{Kind: KindText, Title: "还没有搜索", Text: "先输入“搜索 书名”。"}}, nil
	}
	if !state.HasMore {
		return []Reply{{Kind: KindText, Title: "没有更多结果", Text: "可以换个书名或作者继续搜索。"}}, nil
	}
	var (
		page fanqie.SearchPage
		err  error
	)
	if state.CategoryID != "" {
		provider, ok := state.Provider.(fanqie.CategoryProvider)
		if !ok {
			return []Reply{{Kind: KindText, Title: "分类榜不可用", Text: "当前数据源不支持分类榜。"}}, nil
		}
		page, err = provider.GetCategoryRank(ctx, state.CategoryID, state.CategoryGender, state.NextPage)
	} else if state.Feed != "" {
		page, err = state.Provider.Discover(ctx, state.Feed, state.NextPage)
	} else {
		page, err = state.Provider.Search(ctx, state.Query, state.NextPage)
	}
	if err != nil {
		return nil, err
	}
	state.Books = page.Books
	state.NextPage = page.NextOffset
	state.HasMore = page.HasMore
	if len(page.Books) == 0 {
		state.HasMore = false
		return []Reply{{Kind: KindText, Title: "没有更多结果", Text: "可以换个书名或作者继续搜索。"}}, nil
	}
	return []Reply{{
		Kind:  KindSearch,
		Title: moreTitle(firstNonEmpty(state.CategoryName, state.FeedName), len(page.Books)),
		Text:  searchHint(page.HasMore),
		Books: page.Books,
	}}, nil
}

func moreTitle(feedName string, count int) string {
	if feedName != "" {
		return fmt.Sprintf("%s · 更多 %d 本", feedName, count)
	}
	return fmt.Sprintf("更多结果 · %d 本", count)
}

func searchHint(hasMore bool) string {
	if hasMore {
		return "输入“打开 1”选择，或输入“更多结果”查看下一页。"
	}
	return "输入“打开 1”选择一本书。"
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
	guidance := fmt.Sprintf("目录共 %d 章。可以说“从第 1 章开始读”或“查看目录”。", len(chapters))
	if state.Library != nil {
		if history, ok := state.Library.HistoryFor(book.ID); ok {
			guidance = fmt.Sprintf("目录共 %d 章。上次读到“%s”，输入“继续阅读”恢复，或直接选择其他章节。", len(chapters), history.Chapter.Title)
		}
	}
	if state.officialSessionActive() {
		if provider, ok := state.Provider.(fanqie.BookshelfController); ok {
			if inShelf, checkErr := provider.InBookshelf(ctx, book.ID); checkErr == nil {
				if inShelf {
					guidance += "\n官网书架：已加入；输入“取消收藏”可同步移出。"
				} else {
					guidance += "\n官网书架：尚未加入；输入“收藏”即可同步。"
				}
			}
		}
	}
	return []Reply{
		{Kind: KindBook, Title: "已打开书籍", Book: &book},
		{Kind: KindText, Text: guidance},
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
		return []Reply{{Kind: KindText, Title: entry.Title, Text: "该章节已锁定或需要付费，请使用官方客户端阅读。"}}, nil
	}
	chapter, err := state.Provider.GetChapter(ctx, entry.ID)
	if err != nil {
		if errors.Is(err, fanqie.ErrLocked) {
			return []Reply{{Kind: KindText, Title: entry.Title, Text: fanqie.ErrLocked.Error()}}, nil
		}
		return nil, err
	}
	state.Index = index
	replies := []Reply{{Kind: KindChapter, Title: chapter.Title, Chapter: &chapter}}
	if state.Library != nil {
		if err := state.Library.RecordHistory(*state.Book, entry, index); err != nil {
			replies = append(replies, Reply{Kind: KindText, Title: "历史记录未能保存", Text: err.Error()})
		}
	}
	if state.officialSessionActive() {
		if provider, ok := state.Provider.(fanqie.ProgressController); ok {
			if err := provider.UpdateProgress(ctx, state.Book.ID, entry.ID, entry.Order); err != nil {
				replies = append(replies, Reply{Kind: KindText, Title: "官网进度未能同步", Text: err.Error()})
			}
		}
	}
	return replies, nil
}

func (state *State) addFavorite(ctx context.Context) ([]Reply, error) {
	if state.Book == nil {
		return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先打开一本书，再输入“收藏”。"}}, nil
	}
	if state.officialSessionActive() {
		provider, ok := state.Provider.(fanqie.BookshelfController)
		if !ok {
			return []Reply{{Kind: KindText, Title: "官网书架写入不可用", Text: "当前数据源无法修改官网书架。"}}, nil
		}
		if err := provider.AddToBookshelf(ctx, *state.Book); errors.Is(err, fanqie.ErrLoginRequired) {
			return []Reply{{Kind: KindText, Title: "需要重新登录", Text: state.loginHelp()}}, nil
		} else if err != nil {
			return nil, err
		}
		return []Reply{{Kind: KindText, Title: state.Book.Title, Text: "已加入官网书架；番茄网页与 App 会看到同一状态。"}}, nil
	}
	if state.Library == nil {
		return []Reply{{Kind: KindText, Title: "本地资料库未启用", Text: "当前会话无法保存收藏。"}}, nil
	}
	added, err := state.Library.AddFavorite(*state.Book)
	if err != nil {
		return nil, err
	}
	text := "这本书已经在收藏夹中，书籍信息已刷新。"
	if added {
		text = "已保存到本地收藏夹。"
	}
	return []Reply{{Kind: KindText, Title: state.Book.Title, Text: text}}, nil
}

func (state *State) removeFavorite(ctx context.Context) ([]Reply, error) {
	if state.Book == nil {
		return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先打开要移出收藏夹的书籍。"}}, nil
	}
	if state.officialSessionActive() {
		provider, ok := state.Provider.(fanqie.BookshelfController)
		if !ok {
			return []Reply{{Kind: KindText, Title: "官网书架写入不可用", Text: "当前数据源无法修改官网书架。"}}, nil
		}
		if err := provider.RemoveFromBookshelf(ctx, *state.Book); errors.Is(err, fanqie.ErrLoginRequired) {
			return []Reply{{Kind: KindText, Title: "需要重新登录", Text: state.loginHelp()}}, nil
		} else if err != nil {
			return nil, err
		}
		return []Reply{{Kind: KindText, Title: state.Book.Title, Text: "已从官网书架移出；番茄网页与 App 会同步看到这一状态。"}}, nil
	}
	if state.Library == nil {
		return []Reply{{Kind: KindText, Title: "本地资料库未启用", Text: "当前会话无法修改收藏。"}}, nil
	}
	removed, err := state.Library.RemoveFavorite(state.Book.ID)
	if err != nil {
		return nil, err
	}
	text := "这本书原本不在收藏夹中。"
	if removed {
		text = "已从本地收藏夹移除。"
	}
	return []Reply{{Kind: KindText, Title: state.Book.Title, Text: text}}, nil
}

func (state *State) favorites(ctx context.Context) ([]Reply, error) {
	if state.officialSessionActive() {
		return state.bookshelf(ctx)
	}
	if state.Library == nil {
		return []Reply{{Kind: KindText, Title: "本地资料库未启用", Text: "当前会话没有可用的收藏夹。"}}, nil
	}
	entries := state.Library.Favorites()
	books := make([]fanqie.Book, 0, len(entries))
	for _, entry := range entries {
		book := entry.Book
		if book.Status == "" {
			book.Status = "本地收藏"
		}
		books = append(books, book)
	}
	state.setBookList(books)
	if len(books) == 0 {
		return []Reply{{Kind: KindText, Title: "收藏夹", Text: "收藏夹还是空的。打开一本书后输入“收藏”即可加入。"}}, nil
	}
	return []Reply{{Kind: KindSearch, Title: fmt.Sprintf("收藏夹 · %d 本", len(books)), Text: "输入“打开 1”继续阅读。", Books: books}}, nil
}

func (state *State) history() ([]Reply, error) {
	if state.Library == nil {
		return []Reply{{Kind: KindText, Title: "本地资料库未启用", Text: "当前会话没有可用的阅读历史。"}}, nil
	}
	entries := state.Library.History()
	books := make([]fanqie.Book, 0, len(entries))
	for _, entry := range entries {
		book := entry.Book
		book.Status = "读到 " + entry.Chapter.Title + " · " + entry.ReadAt.Local().Format("01-02 15:04")
		books = append(books, book)
	}
	state.setBookList(books)
	if len(books) == 0 {
		return []Reply{{Kind: KindText, Title: "阅读历史", Text: "还没有阅读记录。读过的章节会自动保存在本机。"}}, nil
	}
	return []Reply{{Kind: KindSearch, Title: fmt.Sprintf("阅读历史 · %d 本", len(books)), Text: "输入“继续阅读”恢复最近进度，或输入“打开 1”。", Books: books}}, nil
}

func (state *State) resume(ctx context.Context) ([]Reply, error) {
	if state.Library == nil {
		return []Reply{{Kind: KindText, Title: "本地资料库未启用", Text: "当前会话无法恢复阅读进度。"}}, nil
	}
	var (
		entry library.HistoryEntry
		ok    bool
	)
	if state.Book != nil {
		entry, ok = state.Library.HistoryFor(state.Book.ID)
		if !ok {
			return []Reply{{Kind: KindText, Title: "当前书籍没有阅读历史", Text: "从任意章节开始阅读后，进度会自动保存在本机。"}}, nil
		}
	}
	if state.Book == nil {
		entry, ok = state.Library.LatestHistory()
	}
	if !ok {
		return []Reply{{Kind: KindText, Title: "没有阅读历史", Text: "打开一本书并开始阅读后，进度会自动保存在本机。"}}, nil
	}
	book, err := state.Provider.GetBook(ctx, entry.Book.ID)
	if err != nil {
		return nil, err
	}
	chapters, err := state.Provider.GetDirectory(ctx, book.ID)
	if err != nil {
		return nil, err
	}
	index := -1
	for position, chapter := range chapters {
		if chapter.ID == entry.Chapter.ID {
			index = position
			break
		}
	}
	if index < 0 && entry.Chapter.Order > 0 {
		for position, chapter := range chapters {
			if chapter.Order == entry.Chapter.Order && (entry.Chapter.Title == "" || chapter.Title == entry.Chapter.Title) {
				index = position
				break
			}
		}
	}
	if index < 0 {
		return []Reply{{Kind: KindText, Title: "无法恢复原章节", Text: "书籍目录已经变化，请重新选择章节。"}}, nil
	}
	if chapters[index].Locked || chapters[index].NeedPay {
		return []Reply{{Kind: KindText, Title: chapters[index].Title, Text: "保存的章节当前已锁定或需要付费，未改变当前阅读位置。"}}, nil
	}
	chapter, err := state.Provider.GetChapter(ctx, chapters[index].ID)
	if err != nil {
		if errors.Is(err, fanqie.ErrLocked) {
			return []Reply{{Kind: KindText, Title: chapters[index].Title, Text: "保存的章节当前不可用，未改变当前阅读位置。"}}, nil
		}
		return nil, err
	}
	state.Book = &book
	state.Chapters = chapters
	state.Index = index
	replies := []Reply{
		{Kind: KindBook, Title: "已恢复阅读", Book: &book},
		{Kind: KindChapter, Title: chapter.Title, Chapter: &chapter},
	}
	if err := state.Library.RecordHistory(book, chapters[index], index); err != nil {
		replies = append(replies, Reply{Kind: KindText, Title: "历史记录未能保存", Text: err.Error()})
	}
	return replies, nil
}

func (state *State) account(ctx context.Context) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.SessionProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "尚未导入登录会话", Text: state.loginHelp()}}, nil
	}
	account, err := provider.GetAccount(ctx)
	if errors.Is(err, fanqie.ErrLoginRequired) {
		return []Reply{{Kind: KindText, Title: "尚未登录或会话已过期", Text: state.loginHelp()}}, nil
	}
	if err != nil {
		return nil, err
	}
	name := account.Name
	if name == "" {
		name = "已登录用户"
	}
	status := "普通账号"
	if !account.VIPKnown {
		status = "账号类型未返回"
	} else if account.VIP {
		status = "VIP 账号"
	}
	sessionDescription := strings.TrimSpace(state.Session.SessionDescription)
	if sessionDescription == "" {
		sessionDescription = "浏览器会话已加载"
	}
	return []Reply{{Kind: KindStatus, Title: "番茄账号", Text: fmt.Sprintf("用户：%s\n状态：%s\n会话：%s\n\n输入 /logout 可清除本机登录状态。", name, status, sessionDescription)}}, nil
}

func (state *State) logout(ctx context.Context) ([]Reply, error) {
	if state.Session.Logout != nil {
		if err := state.Session.Logout(ctx); err != nil {
			return nil, err
		}
		return logoutReply(), nil
	}
	if provider, ok := state.Provider.(interface{ Logout(context.Context) error }); ok {
		if err := provider.Logout(ctx); err != nil {
			return nil, err
		}
		return logoutReply(), nil
	}
	if provider, ok := state.Provider.(interface{ Logout() error }); ok {
		if err := provider.Logout(); err != nil {
			return nil, err
		}
		return logoutReply(), nil
	}
	if provider, ok := state.Provider.(interface{ ClearSession() }); ok {
		provider.ClearSession()
		return logoutReply(), nil
	}
	if provider, ok := state.Provider.(interface{ ClearSession() error }); ok {
		if err := provider.ClearSession(); err != nil {
			return nil, err
		}
		return logoutReply(), nil
	}
	return []Reply{{
		Kind:  KindText,
		Title: "当前版本无法在界面内退出",
		Text:  "请退出 fanqie 后删除默认配置目录中的 Cookie 文件，再重新启动。输入 /login 可查看默认路径与登录步骤。",
	}}, nil
}

func logoutReply() []Reply {
	return []Reply{{
		Kind:  KindStatus,
		Title: "已退出登录",
		Text:  "本机保存的登录会话已清除。公开搜索与阅读仍可继续使用；需要账号功能时可再次输入 /login。",
	}}
}

func (state *State) cloudHistory(ctx context.Context) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.CloudProgressProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "云端进度不可用", Text: loginCapabilityHelp("当前数据源没有实现官网阅读进度接口。")}}, nil
	}
	items, err := provider.GetCloudProgress(ctx)
	if errors.Is(err, fanqie.ErrLoginRequired) {
		return []Reply{{Kind: KindText, Title: "需要登录", Text: state.loginHelp()}}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []Reply{{Kind: KindText, Title: "云端阅读进度", Text: "账号当前没有返回可用的云端阅读进度。"}}, nil
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("• 书籍 %s · %s · %s%s", displayID(item.BookID), cloudChapterLabel(item.ChapterID, item.ChapterOrder), progressLabel(item.ReadProgress, item.ProgressKnown), timeLabel(item.UpdatedAt)))
	}
	return []Reply{{
		Kind:  KindStatus,
		Title: fmt.Sprintf("云端阅读进度 · %d 条", len(items)),
		Text:  strings.Join(lines, "\n"),
	}}, nil
}

func (state *State) readItems(ctx context.Context) ([]Reply, error) {
	if state.Book == nil {
		return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先搜索并打开一本书，再输入 /read-items 查看该书的官网已读记录。"}}, nil
	}
	provider, ok := state.Provider.(fanqie.ReadItemsProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "官网已读记录不可用", Text: loginCapabilityHelp("当前数据源没有实现官网已读记录接口。")}}, nil
	}
	items, err := provider.GetReadItems(ctx, state.Book.ID)
	if errors.Is(err, fanqie.ErrLoginRequired) {
		return []Reply{{Kind: KindText, Title: "需要登录", Text: state.loginHelp()}}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []Reply{{Kind: KindText, Title: state.Book.Title + " · 官网已读记录", Text: "账号当前没有返回这本书的已读记录。"}}, nil
	}
	lines := make([]string, 0, len(items))
	for index, item := range items {
		lines = append(lines, fmt.Sprintf("%d. %s · %s%s", index+1, state.readItemLabel(item), progressLabel(item.ReadProgress, item.ProgressKnown), timeLabel(item.UpdatedAt)))
	}
	return []Reply{{
		Kind:  KindStatus,
		Title: fmt.Sprintf("%s · 官网已读记录 %d 条", state.Book.Title, len(items)),
		Text:  strings.Join(lines, "\n"),
	}}, nil
}

func (state *State) bookshelf(ctx context.Context) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.BookshelfProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "官网书架不可用", Text: loginCapabilityHelp("当前数据源没有实现官网书架接口。")}}, nil
	}
	books, err := provider.GetBookshelf(ctx)
	if errors.Is(err, fanqie.ErrLoginRequired) {
		return []Reply{{Kind: KindText, Title: "需要登录", Text: state.loginHelp()}}, nil
	}
	if err != nil {
		return nil, err
	}
	state.setBookList(books)
	if len(books) == 0 {
		return []Reply{{Kind: KindText, Title: "官网书架", Text: "账号当前的官网书架为空，或官网没有返回可展示的书籍。"}}, nil
	}
	return []Reply{{Kind: KindSearch, Title: fmt.Sprintf("官网书架 · %d 本", len(books)), Text: "登录后收藏与这里同步。输入“打开 1”开始阅读。", Books: books}}, nil
}

func (state *State) syncAccount(ctx context.Context) ([]Reply, error) {
	if !state.officialSessionActive() {
		return []Reply{{Kind: KindText, Title: "同步需要登录", Text: state.loginHelp()}}, nil
	}
	sessionProvider, ok := state.Provider.(fanqie.SessionProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "账号同步不可用", Text: "当前数据源没有实现官网账号接口。"}}, nil
	}
	account, err := sessionProvider.GetAccount(ctx)
	if errors.Is(err, fanqie.ErrLoginRequired) {
		return []Reply{{Kind: KindText, Title: "需要重新登录", Text: state.loginHelp()}}, nil
	}
	if err != nil {
		return nil, err
	}

	lines := []string{"账号：" + firstNonEmpty(account.Name, "已登录用户")}
	books := []fanqie.Book(nil)
	if provider, available := state.Provider.(fanqie.BookshelfProvider); available {
		books, err = provider.GetBookshelf(ctx)
		if errors.Is(err, fanqie.ErrLoginRequired) {
			return []Reply{{Kind: KindText, Title: "需要重新登录", Text: state.loginHelp()}}, nil
		}
		if err != nil {
			return nil, err
		}
		state.setBookList(books)
		lines = append(lines, fmt.Sprintf("官网书架：%d 本", len(books)))
	} else {
		lines = append(lines, "官网书架：当前数据源不支持")
	}
	if provider, available := state.Provider.(fanqie.CloudProgressProvider); available {
		progress, progressErr := provider.GetCloudProgress(ctx)
		if errors.Is(progressErr, fanqie.ErrLoginRequired) {
			return []Reply{{Kind: KindText, Title: "需要重新登录", Text: state.loginHelp()}}, nil
		}
		if progressErr != nil {
			return nil, progressErr
		}
		lines = append(lines, fmt.Sprintf("云端进度：%d 条", len(progress)))
	} else {
		lines = append(lines, "云端进度：当前数据源不支持")
	}

	replies := []Reply{{Kind: KindStatus, Title: "官网数据已同步", Text: strings.Join(lines, "\n")}}
	if len(books) > 0 {
		replies = append(replies, Reply{Kind: KindSearch, Title: fmt.Sprintf("官网书架 · %d 本", len(books)), Text: "书架结果已载入，输入“打开 1”即可阅读。", Books: books})
	}
	return replies, nil
}

func (state *State) officialSessionActive() bool {
	controller, ok := state.Provider.(fanqie.SessionController)
	return ok && controller.HasSession()
}

func (state *State) reviews(ctx context.Context, currentBookOnly bool) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.CommentProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "官网书评不可用", Text: "当前数据源没有实现官网公开书评接口。"}}, nil
	}
	feed, err := provider.GetReviewFeed(ctx)
	if err != nil {
		return nil, err
	}
	if currentBookOnly {
		if state.Book == nil {
			return []Reply{{Kind: KindText, Title: "还没有打开书籍", Text: "先打开一本书，再输入“书评”；也可输入“最新书评”查看官网公开索引。"}}, nil
		}
		filtered := make([]fanqie.ReviewSummary, 0)
		for _, review := range feed {
			if review.BookID == state.Book.ID {
				filtered = append(filtered, review)
			}
		}
		feed = filtered
	}
	if len(feed) == 0 {
		state.Reviews = nil
		title := "本书书评"
		if state.Book != nil {
			title = state.Book.Title + " · 书评"
		}
		return []Reply{{Kind: KindText, Title: title, Text: "官网 PC 端没有提供完整的按书分页接口，这本书未出现在当前公开书评索引中。书籍聚合评分和阅读量仍会显示在详情页；输入“最新书评”可浏览官网最近公开的书评。"}}, nil
	}
	limit := min(20, len(feed))
	state.Reviews = append([]fanqie.ReviewSummary(nil), feed[:limit]...)
	lines := make([]string, 0, limit+2)
	for index, review := range feed[:limit] {
		bookTitle := firstNonEmpty(review.BookTitle, "书籍 "+review.BookID)
		lines = append(lines, fmt.Sprintf("%d. %s · %s", index+1, bookTitle, truncateRunes(review.Text, 54)))
	}
	if len(feed) > limit {
		lines = append(lines, fmt.Sprintf("…当前展示前 %d 条", limit))
	}
	lines = append(lines, "", "输入“书评 1”查看正文、评分与回复；也支持 /comment bookID-commentID。")
	title := fmt.Sprintf("官网最新书评 · %d 条", len(feed))
	if currentBookOnly {
		title = fmt.Sprintf("%s · 当前公开书评 %d 条", state.Book.Title, len(feed))
	}
	return []Reply{{Kind: KindText, Title: title, Text: strings.Join(lines, "\n")}}, nil
}

func (state *State) comment(ctx context.Context, input string) ([]Reply, error) {
	provider, ok := state.Provider.(fanqie.CommentProvider)
	if !ok {
		return []Reply{{Kind: KindText, Title: "官网书评不可用", Text: "当前数据源没有实现官网书评详情接口。"}}, nil
	}
	bookID, commentID := "", ""
	if position, err := strconv.Atoi(input); err == nil && position >= 1 && position <= len(state.Reviews) {
		bookID, commentID = state.Reviews[position-1].BookID, state.Reviews[position-1].CommentID
	} else if matches := commentURL.FindStringSubmatch(strings.TrimSpace(input)); len(matches) == 3 {
		bookID, commentID = matches[1], matches[2]
	} else if parts := strings.Split(input, "-"); len(parts) == 2 {
		bookID, commentID = parts[0], parts[1]
	} else if state.Book != nil {
		bookID, commentID = state.Book.ID, input
	}
	if bookID == "" || commentID == "" {
		return []Reply{{Kind: KindText, Title: "找不到这条书评", Text: "先输入“最新书评”，再输入“书评 1”；也可输入 /comment bookID-commentID。"}}, nil
	}
	detail, err := provider.GetComment(ctx, bookID, commentID)
	if err != nil {
		return nil, err
	}
	replies := []Reply{{Kind: KindBook, Title: "书评所属书籍", Book: &detail.Book}}
	for index, review := range detail.Reviews {
		title := "书评"
		if index > 0 {
			title = "追评"
		}
		replies = append(replies, Reply{Kind: KindText, Title: title + " · " + firstNonEmpty(review.UserName, "匿名读者"), Text: reviewText(review)})
	}
	if len(detail.Replies) > 0 {
		lines := make([]string, 0, len(detail.Replies))
		for _, review := range detail.Replies {
			lines = append(lines, fmt.Sprintf("%s：%s%s", firstNonEmpty(review.UserName, "匿名读者"), review.Text, likesLabel(review.Likes)))
		}
		replies = append(replies, Reply{Kind: KindText, Title: fmt.Sprintf("回复 · %d 条", len(detail.Replies)), Text: strings.Join(lines, "\n\n")})
	}
	return replies, nil
}

func reviewText(review fanqie.Review) string {
	meta := make([]string, 0, 4)
	if review.ScoreKnown {
		meta = append(meta, fmt.Sprintf("评分 %.1f / 10", review.Score))
	}
	if review.ReadDuration > 0 {
		meta = append(meta, "阅读时长 "+review.ReadDuration.Round(time.Minute).String())
	}
	if review.Likes > 0 {
		meta = append(meta, fmt.Sprintf("%d 赞", review.Likes))
	}
	if review.ReplyCount > 0 {
		meta = append(meta, fmt.Sprintf("%d 回复", review.ReplyCount))
	}
	if len(meta) == 0 {
		return review.Text
	}
	return strings.Join(meta, " · ") + "\n\n" + review.Text
}

func likesLabel(likes int) string {
	if likes <= 0 {
		return ""
	}
	return fmt.Sprintf(" · %d 赞", likes)
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func cloudChapterLabel(chapterID string, order int) string {
	if order > 0 {
		return fmt.Sprintf("第 %d 章", order)
	}
	if chapterID = strings.TrimSpace(chapterID); chapterID != "" {
		return "章节 " + chapterID
	}
	return "章节未知"
}

func (state *State) readItemLabel(item fanqie.ReadItem) string {
	for _, chapter := range state.Chapters {
		if chapter.ID == item.ChapterID {
			return chapter.Title
		}
	}
	return cloudChapterLabel(item.ChapterID, item.ChapterOrder)
}

func loginCapabilityHelp(reason string) string {
	return reason + " 可输入 /account 检查会话，或输入 /login 查看登录步骤。公开搜索和本地阅读历史不受影响。"
}

func displayID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "未知"
	}
	return value
}

func progressLabel(progress float64, known bool) string {
	if !known || progress < 0 {
		return "进度未知"
	}
	if progress <= 1 {
		progress *= 100
	}
	if progress > 100 {
		progress = 100
	}
	return fmt.Sprintf("进度 %.0f%%", progress)
}

func timeLabel(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return " · " + value.Local().Format("2006-01-02 15:04")
}

func (state *State) setBookList(books []fanqie.Book) {
	state.Books = books
	state.Query = ""
	state.Feed = ""
	state.FeedName = ""
	state.clearCategory()
	state.NextPage = 0
	state.HasMore = false
}

func (state *State) clearCategory() {
	state.CategoryID = ""
	state.CategoryGender = ""
	state.CategoryName = ""
}

func genderName(gender string) string {
	if gender == "female" {
		return "女频"
	}
	return "男频"
}

func nonEmptyAgent(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func labelValue(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return label + "：" + value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
		"  热门榜 / 编辑推荐 / 男频精选 / 女频精选 / 最近更新 / 出版榜\n  分类 / 分类 男 西方奇幻\n  热门作者 / 作者 1\n  搜索 三体\n  更多结果\n  打开 1\n  从第 3 章开始读\n  下一章 / 上一章\n  查看目录\n  收藏 / 取消收藏 / 官网书架\n  同步账号\n  书评 / 最新书评 / 书评 1\n  历史记录 / 继续阅读\n  登录状态\n  现在读到哪\n\n" +
		"斜杠命令：\n" +
		"  /popular  /recommend  /male  /female  /updates  /published\n" +
		"  /categories  /category female 古风世情\n" +
		"  /authors  /author 1\n  /search  /more  /open  /read  /catalog\n" +
		"  /next  /prev  /favorite  /unfavorite  /favorites\n" +
		"  /history  /resume  /account  /login\n" +
		"  /bookshelf  /cloud-history  /read-items  /sync  /logout\n" +
		"  /reviews  /review-feed  /comment bookID-commentID（也可粘贴官网书评链接）\n" +
		"  /font standard|bold|relaxed（也可按 F2）\n" +
		"  /status  /clear  /quit"
}

func (state *State) loginHelp() string {
	if guidance := strings.TrimSpace(state.Session.LoginGuide); guidance != "" {
		return guidance
	}
	return "本程序不接收账号密码，请通过浏览器 Cookie 安全登录：\n\n" +
		"  1. 在浏览器登录 fanqienovel.com。\n" +
		"  2. 导出 Cookie 到当前目录的 fanqienovel.com_cookies.txt。\n" +
		"  3. 执行 chmod 600 ./fanqienovel.com_cookies.txt。\n" +
		"  4. 运行 fanqie -cookie-file ./fanqienovel.com_cookies.txt。\n\n" +
		"验证成功后，程序会将 Cookie 安全保存到默认配置目录；以后直接运行 fanqie 即可自动登录。也可把 Cookie 文件放在当前目录，让程序启动时自动发现。Cookie 等同登录凭据，请勿粘贴到聊天、截图或提交到 Git。"
}
