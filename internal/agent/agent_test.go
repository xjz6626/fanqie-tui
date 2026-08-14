package agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xjz6626/fanqie-tui/internal/fanqie"
	"github.com/xjz6626/fanqie-tui/internal/library"
)

type fakeProvider struct{}

type runtimeLockedProvider struct{ fakeProvider }

type shiftedDirectoryProvider struct{ fakeProvider }

func (shiftedDirectoryProvider) GetDirectory(context.Context, string) ([]fanqie.Chapter, error) {
	return []fanqie.Chapter{
		{ID: "inserted", Title: "插入章节", Order: 0},
		{ID: "renamed-2", Title: "第2章 后续", Order: 2},
	}, nil
}

type changedDirectoryProvider struct{ fakeProvider }

func (changedDirectoryProvider) GetDirectory(context.Context, string) ([]fanqie.Chapter, error) {
	return []fanqie.Chapter{{ID: "different", Title: "不同章节", Order: 99}}, nil
}

type resumeLockedProvider struct{ fakeProvider }

func (resumeLockedProvider) GetDirectory(context.Context, string) ([]fanqie.Chapter, error) {
	return []fanqie.Chapter{{ID: "chapter-2", Title: "第2章 后续", Order: 2, Locked: true}}, nil
}

type fakeSessionProvider struct{ fakeProvider }

func (fakeSessionProvider) GetAccount(context.Context) (fanqie.Account, error) {
	return fanqie.Account{ID: "user-1", Name: "读者", VIP: true, VIPKnown: true}, nil
}

type fakeAuthenticatedProvider struct{ fakeSessionProvider }

func (fakeAuthenticatedProvider) HasSession() bool  { return true }
func (fakeAuthenticatedProvider) SetSession(string) {}
func (fakeAuthenticatedProvider) ClearSession()     {}

func (fakeAuthenticatedProvider) GetCloudProgress(context.Context) ([]fanqie.CloudProgress, error) {
	return []fanqie.CloudProgress{{
		BookID:        "book-1",
		ChapterID:     "chapter-2",
		ChapterOrder:  2,
		ReadProgress:  0.75,
		ProgressKnown: true,
		UpdatedAt:     time.Date(2026, time.August, 13, 12, 30, 0, 0, time.Local),
	}}, nil
}

func (fakeAuthenticatedProvider) GetReadItems(_ context.Context, bookID string) ([]fanqie.ReadItem, error) {
	if bookID != "book-1" {
		return nil, nil
	}
	return []fanqie.ReadItem{{ChapterID: "chapter-2", ChapterOrder: 2, ReadProgress: 42, ProgressKnown: true}}, nil
}

func (fakeAuthenticatedProvider) GetBookshelf(context.Context) ([]fanqie.Book, error) {
	return []fanqie.Book{{ID: "shelf-1", Title: "官网藏书", Status: "官网书架"}}, nil
}

func (fakeAuthenticatedProvider) InBookshelf(context.Context, string) (bool, error) {
	return false, nil
}

func (fakeAuthenticatedProvider) AddToBookshelf(context.Context, fanqie.Book) error { return nil }

func (fakeAuthenticatedProvider) RemoveFromBookshelf(context.Context, fanqie.Book) error { return nil }

func (fakeAuthenticatedProvider) UpdateProgress(context.Context, string, string, int) error {
	return nil
}

func (fakeAuthenticatedProvider) GetReviewFeed(context.Context) ([]fanqie.ReviewSummary, error) {
	return []fanqie.ReviewSummary{{BookID: "book-1", CommentID: "comment-1", BookTitle: "示例书", Text: "很好看"}}, nil
}

func (fakeAuthenticatedProvider) GetComment(context.Context, string, string) (fanqie.CommentDetail, error) {
	return fanqie.CommentDetail{
		Book:    fanqie.Book{ID: "book-1", Title: "示例书", Score: 9.2, ReadCount: 12000},
		Reviews: []fanqie.Review{{ID: "comment-1", UserName: "读者", Text: "完整书评", Score: 10, ScoreKnown: true, Likes: 8}},
		Replies: []fanqie.Review{{ID: "reply-1", UserName: "另一位读者", Text: "同意", Likes: 2}},
	}, nil
}

func (fakeAuthenticatedProvider) GetCategories(context.Context) ([]fanqie.Category, error) {
	return []fanqie.Category{
		{ID: "1141", Name: "西方奇幻", Gender: "male"},
		{ID: "8", Name: "科幻末世", Gender: "male"},
		{ID: "8", Name: "科幻末世", Gender: "female"},
		{ID: "1139", Name: "古风世情", Gender: "female"},
	}, nil
}

func (fakeAuthenticatedProvider) GetCategoryRank(_ context.Context, categoryID, gender string, offset int) (fanqie.SearchPage, error) {
	return fanqie.SearchPage{
		Books:      []fanqie.Book{{ID: "category-book", Title: categoryID + "分类书", Author: gender}},
		NextOffset: offset + 1,
		HasMore:    offset == 0,
	}, nil
}

func (fakeAuthenticatedProvider) GetTopAuthors(context.Context) ([]fanqie.Author, error) {
	return []fanqie.Author{{ID: "author-10000", Name: "空留", Level: "殿堂作家", Introduction: "代表作"}}, nil
}

func (fakeAuthenticatedProvider) GetAuthor(_ context.Context, authorID string) (fanqie.AuthorProfile, []fanqie.Book, error) {
	return fanqie.AuthorProfile{Author: fanqie.Author{ID: authorID, Name: "空留", Introduction: "作者简介"}, Followers: "45万"},
		[]fanqie.Book{{ID: "author-book", Title: "作者作品", Author: "空留"}}, nil
}

type expiredCloudProvider struct{ fakeProvider }

func (expiredCloudProvider) GetCloudProgress(context.Context) ([]fanqie.CloudProgress, error) {
	return nil, fanqie.ErrLoginRequired
}

type fakeLogoutProvider struct {
	fakeProvider
	loggedOut bool
}

func (provider *fakeLogoutProvider) Logout(context.Context) error {
	provider.loggedOut = true
	return nil
}

func (runtimeLockedProvider) GetChapter(context.Context, string) (fanqie.ChapterContent, error) {
	return fanqie.ChapterContent{}, fanqie.ErrLocked
}

func (fakeProvider) Search(_ context.Context, query string, offset int) (fanqie.SearchPage, error) {
	if offset > 0 {
		return fanqie.SearchPage{Books: []fanqie.Book{{ID: "book-2", Title: query + "更多结果", Author: "作者"}}}, nil
	}
	return fanqie.SearchPage{
		Books:      []fanqie.Book{{ID: "book-1", Title: query + "结果", Author: "作者"}},
		NextOffset: 10,
		HasMore:    true,
	}, nil
}

func (fakeProvider) Discover(_ context.Context, kind fanqie.DiscoverKind, offset int) (fanqie.SearchPage, error) {
	title := map[fanqie.DiscoverKind]string{
		fanqie.DiscoverPopular:     "热门书",
		fanqie.DiscoverRecommended: "推荐书",
		fanqie.DiscoverMale:        "男频书",
		fanqie.DiscoverFemale:      "女频书",
		fanqie.DiscoverRecent:      "更新书",
		fanqie.DiscoverPublished:   "出版书",
	}[kind]
	return fanqie.SearchPage{
		Books:      []fanqie.Book{{ID: "discover-1", Title: title}},
		NextOffset: offset + 10,
		HasMore:    kind == fanqie.DiscoverPublished && offset == 0,
	}, nil
}

func (fakeProvider) GetBook(_ context.Context, id string) (fanqie.Book, error) {
	return fanqie.Book{ID: id, Title: "示例书", Author: "作者", ChapterCount: 3}, nil
}

func (fakeProvider) GetDirectory(context.Context, string) ([]fanqie.Chapter, error) {
	return []fanqie.Chapter{
		{ID: "chapter-1", Title: "第1章 开篇", Order: 1},
		{ID: "chapter-2", Title: "第2章 后续", Order: 2},
		{ID: "chapter-3", Title: "第3章 锁定", Order: 3, Locked: true},
	}, nil
}

func (fakeProvider) GetChapter(_ context.Context, id string) (fanqie.ChapterContent, error) {
	order := 1
	if id == "chapter-2" {
		order = 2
	}
	return fanqie.ChapterContent{BookID: "book-1", ChapterID: id, Title: "正文标题", Order: order, Content: "章节正文"}, nil
}

func TestConversationalFlow(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()

	replies, err := state.Handle(ctx, "搜索 示例")
	if err != nil || len(replies) != 1 || replies[0].Kind != KindSearch {
		t.Fatalf("search: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "打开 1")
	if err != nil || state.Book == nil || len(state.Chapters) != 3 || replies[0].Kind != KindBook {
		t.Fatalf("open: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "从第 1 章开始读")
	if err != nil || replies[0].Kind != KindChapter || state.Index != 0 {
		t.Fatalf("read: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "下一章")
	if err != nil || replies[0].Chapter == nil || replies[0].Chapter.Order != 2 || state.Index != 1 {
		t.Fatalf("next: replies=%+v err=%v", replies, err)
	}
	replies, err = state.Handle(ctx, "上一章")
	if err != nil || state.Index != 0 {
		t.Fatalf("prev: replies=%+v err=%v", replies, err)
	}
}

func TestBarePromptSearchesBeforeBookIsOpen(t *testing.T) {
	state := New(fakeProvider{})
	replies, err := state.Handle(context.Background(), "三体")
	if err != nil || replies[0].Books[0].Title != "三体结果" {
		t.Fatalf("replies=%+v err=%v", replies, err)
	}
}

func TestSearchPagination(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()
	_, err := state.Handle(ctx, "搜索 三体")
	if err != nil || state.Query != "三体" || state.NextPage != 10 || !state.HasMore {
		t.Fatalf("query=%q next=%d more=%v err=%v", state.Query, state.NextPage, state.HasMore, err)
	}
	replies, err := state.Handle(ctx, "更多结果")
	if err != nil || len(replies) != 1 || replies[0].Kind != KindSearch || state.HasMore {
		t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
	}
	if got := replies[0].Books[0].Title; got != "三体更多结果" {
		t.Fatalf("got %q", got)
	}
	replies, err = state.Handle(ctx, "/more")
	if err != nil || replies[0].Title != "没有更多结果" {
		t.Fatalf("replies=%+v err=%v", replies, err)
	}
}

func TestDiscoveryFeedsAndPagination(t *testing.T) {
	tests := []struct {
		command string
		kind    fanqie.DiscoverKind
		title   string
	}{
		{command: "热门榜", kind: fanqie.DiscoverPopular, title: "热门书"},
		{command: "/recommend", kind: fanqie.DiscoverRecommended, title: "推荐书"},
		{command: "男频精选", kind: fanqie.DiscoverMale, title: "男频书"},
		{command: "/female", kind: fanqie.DiscoverFemale, title: "女频书"},
		{command: "最近更新", kind: fanqie.DiscoverRecent, title: "更新书"},
		{command: "出版榜", kind: fanqie.DiscoverPublished, title: "出版书"},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			state := New(fakeProvider{})
			replies, err := state.Handle(context.Background(), test.command)
			if err != nil || len(replies) != 1 || replies[0].Kind != KindSearch || state.Feed != test.kind {
				t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
			}
			if replies[0].Books[0].Title != test.title {
				t.Fatalf("unexpected books: %+v", replies[0].Books)
			}
			if test.kind == fanqie.DiscoverPublished {
				replies, err = state.Handle(context.Background(), "更多结果")
				if err != nil || !strings.Contains(replies[0].Title, "出版榜") || state.HasMore {
					t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
				}
			}
		})
	}
}

func TestPersistentHistoryFavoritesAndResume(t *testing.T) {
	local, err := library.Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := NewWithLibrary(fakeProvider{}, local)
	ctx := context.Background()
	_, _ = state.Handle(ctx, "示例")
	_, _ = state.Handle(ctx, "打开 1")
	if _, err := state.Handle(ctx, "第 2 章"); err != nil {
		t.Fatal(err)
	}
	if got := local.History(); len(got) != 1 || got[0].Chapter.ID != "chapter-2" {
		t.Fatalf("history=%+v", got)
	}
	if replies, err := state.Handle(ctx, "收藏"); err != nil || !local.IsFavorite("book-1") || !strings.Contains(replies[0].Text, "已保存") {
		t.Fatalf("favorite replies=%+v err=%v", replies, err)
	}
	if replies, err := state.Handle(ctx, "收藏夹"); err != nil || replies[0].Kind != KindSearch || len(state.Books) != 1 {
		t.Fatalf("favorites replies=%+v err=%v", replies, err)
	}
	if replies, err := state.Handle(ctx, "历史记录"); err != nil || replies[0].Kind != KindSearch || !strings.Contains(state.Books[0].Status, "第2章") {
		t.Fatalf("history replies=%+v books=%+v err=%v", replies, state.Books, err)
	}
	state.Book = nil
	state.Chapters = nil
	state.Index = -1
	if replies, err := state.Handle(ctx, "继续阅读"); err != nil || state.Index != 1 || len(replies) < 2 || replies[0].Kind != KindBook {
		t.Fatalf("resume replies=%+v state=%+v err=%v", replies, state, err)
	}
	if replies, err := state.Handle(ctx, "取消收藏"); err != nil || local.IsFavorite("book-1") || !strings.Contains(replies[0].Text, "已从") {
		t.Fatalf("remove replies=%+v err=%v", replies, err)
	}
}

func TestResumeMatchesStableOrderAndTitleAfterDirectoryShift(t *testing.T) {
	local, err := library.Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := local.RecordHistory(fanqie.Book{ID: "book-1", Title: "示例书"}, fanqie.Chapter{ID: "old-2", Title: "第2章 后续", Order: 2}, 1); err != nil {
		t.Fatal(err)
	}
	state := NewWithLibrary(shiftedDirectoryProvider{}, local)
	replies, err := state.Handle(context.Background(), "继续阅读")
	if err != nil || state.Index != 1 || len(replies) < 2 || replies[1].Kind != KindChapter {
		t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
	}
}

func TestResumeDirectoryChangeDoesNotGuessByOldIndex(t *testing.T) {
	local, err := library.Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := local.RecordHistory(fanqie.Book{ID: "book-1", Title: "示例书"}, fanqie.Chapter{ID: "old", Title: "旧章节", Order: 2}, 0); err != nil {
		t.Fatal(err)
	}
	state := NewWithLibrary(changedDirectoryProvider{}, local)
	replies, err := state.Handle(context.Background(), "继续阅读")
	if err != nil || state.Book != nil || state.Index != -1 || !strings.Contains(replies[0].Title, "无法恢复") {
		t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
	}
}

func TestResumeLockedDoesNotReplaceCurrentSession(t *testing.T) {
	local, err := library.Open(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := local.RecordHistory(fanqie.Book{ID: "book-1", Title: "示例书"}, fanqie.Chapter{ID: "chapter-2", Title: "第2章 后续", Order: 2}, 1); err != nil {
		t.Fatal(err)
	}
	current := fanqie.Book{ID: "current", Title: "当前书"}
	state := NewWithLibrary(resumeLockedProvider{}, local)
	state.Book = &current
	// Current book has no history, so resume should not jump to another book.
	replies, err := state.Handle(context.Background(), "继续阅读")
	if err != nil || state.Book.ID != "current" || !strings.Contains(replies[0].Title, "当前书籍") {
		t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
	}

	state.Book = nil
	replies, err = state.Handle(context.Background(), "继续阅读")
	if err != nil || state.Book != nil || state.Index != -1 || !strings.Contains(replies[0].Text, "未改变") {
		t.Fatalf("replies=%+v state=%+v err=%v", replies, state, err)
	}
}

func TestAccountStatus(t *testing.T) {
	state := New(fakeSessionProvider{})
	ctx := context.Background()
	replies, err := state.Handle(ctx, "登录状态")
	if err != nil || !strings.Contains(replies[0].Text, "读者") || !strings.Contains(replies[0].Text, "VIP") {
		t.Fatalf("account replies=%+v err=%v", replies, err)
	}
}

func TestLoginGuidanceDoesNotRequestPassword(t *testing.T) {
	state := New(fakeProvider{})
	for _, input := range []string{"登录", "登录状态"} {
		replies, err := state.Handle(context.Background(), input)
		if err != nil || len(replies) != 1 || !strings.Contains(replies[0].Text, "-cookie-file ./fanqienovel.com_cookies.txt") || !strings.Contains(replies[0].Text, "自动登录") || strings.Contains(replies[0].Text, "输入密码") {
			t.Fatalf("%s replies=%+v err=%v", input, replies, err)
		}
	}
}

func TestCustomPersistentSessionGuidanceAndAccountDescription(t *testing.T) {
	state := New(fakeSessionProvider{}).WithSessionActions(SessionActions{
		LoginGuide:         "自定义安全导入说明",
		SessionDescription: "已从默认配置自动加载",
	})
	login, err := state.Handle(context.Background(), "/login")
	if err != nil || login[0].Text != "自定义安全导入说明" {
		t.Fatalf("login=%+v err=%v", login, err)
	}
	account, err := state.Handle(context.Background(), "/account")
	if err != nil || !strings.Contains(account[0].Text, "已从默认配置自动加载") || strings.Contains(account[0].Text, "sessionid") {
		t.Fatalf("account=%+v err=%v", account, err)
	}
}

func TestLoginUsesConfiguredCurrentDirectoryImporter(t *testing.T) {
	state := New(fakeProvider{}).WithSessionActions(SessionActions{Login: func(context.Context) (string, error) {
		return "已验证并保存到默认配置", nil
	}})
	replies, err := state.Handle(context.Background(), "登录")
	if err != nil || !strings.Contains(replies[0].Title, "成功") || !strings.Contains(replies[0].Text, "已验证") {
		t.Fatalf("replies=%+v err=%v", replies, err)
	}

	state.Session.Login = func(context.Context) (string, error) { return "", errors.New("没有找到 Cookie 文件") }
	replies, err = state.Handle(context.Background(), "/login")
	if err != nil || !strings.Contains(replies[0].Title, "未完成") || !strings.Contains(replies[0].Text, "没有找到") || strings.Contains(replies[0].Text, "sessionid") {
		t.Fatalf("replies=%+v err=%v", replies, err)
	}
}

func TestLogoutUsesConfiguredActionWithoutDisplayingSecret(t *testing.T) {
	called := false
	state := New(fakeProvider{}).WithSessionActions(SessionActions{Logout: func(context.Context) error {
		called = true
		return nil
	}})
	replies, err := state.Handle(context.Background(), "/logout")
	if err != nil || !called || !strings.Contains(replies[0].Title, "已退出") || strings.Contains(replies[0].Text, "Cookie:") {
		t.Fatalf("called=%v replies=%+v err=%v", called, replies, err)
	}
}

func TestLogoutSupportsProviderCapabilityAndAnonymousFallback(t *testing.T) {
	provider := &fakeLogoutProvider{}
	state := New(provider)
	if replies, err := state.Handle(context.Background(), "退出登录"); err != nil || !provider.loggedOut || !strings.Contains(replies[0].Title, "已退出") {
		t.Fatalf("loggedOut=%v replies=%+v err=%v", provider.loggedOut, replies, err)
	}

	replies, err := New(fakeProvider{}).Handle(context.Background(), "/logout")
	if err != nil || !strings.Contains(replies[0].Text, "/login") {
		t.Fatalf("fallback replies=%+v err=%v", replies, err)
	}
}

func TestAuthenticatedReadingCommands(t *testing.T) {
	state := New(fakeAuthenticatedProvider{})
	cloud, err := state.Handle(context.Background(), "/cloud-history")
	if err != nil || !strings.Contains(cloud[0].Title, "1 条") || !strings.Contains(cloud[0].Text, "书籍 book-1") || !strings.Contains(cloud[0].Text, "75%") || !strings.Contains(cloud[0].Text, "2026-08-13") {
		t.Fatalf("cloud=%+v err=%v", cloud, err)
	}

	missingBook, err := state.Handle(context.Background(), "/read-items")
	if err != nil || !strings.Contains(missingBook[0].Title, "还没有") {
		t.Fatalf("missingBook=%+v err=%v", missingBook, err)
	}
	_, _ = state.Handle(context.Background(), "搜索 示例")
	_, _ = state.Handle(context.Background(), "打开 1")
	items, err := state.Handle(context.Background(), "官网已读")
	if err != nil || !strings.Contains(items[0].Title, "1 条") || !strings.Contains(items[0].Text, "第2章") || !strings.Contains(items[0].Text, "42%") {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestAuthenticatedReadingCommandsDoNotDisplayChapterZero(t *testing.T) {
	provider := fakeAuthenticatedProvider{}
	state := New(provider)
	state.Book = &fanqie.Book{ID: "book-1", Title: "示例书"}
	state.Chapters = []fanqie.Chapter{{ID: "chapter-2", Title: "第二章"}}
	items, err := state.Handle(context.Background(), "/read-items")
	if err != nil || strings.Contains(items[0].Text, "第 0 章") || !strings.Contains(items[0].Text, "第二章") {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestProgressLabelDistinguishesZeroFromUnknown(t *testing.T) {
	if got := progressLabel(0, true); got != "进度 0%" {
		t.Fatalf("known zero progress=%q", got)
	}
	if got := progressLabel(0, false); got != "进度未知" {
		t.Fatalf("unknown progress=%q", got)
	}
}

func TestAuthenticatedReadingCommandsExplainUnavailableStates(t *testing.T) {
	unsupported, err := New(fakeProvider{}).Handle(context.Background(), "/cloud-history")
	if err != nil || !strings.Contains(unsupported[0].Text, "/account") {
		t.Fatalf("unsupported=%+v err=%v", unsupported, err)
	}
	expired, err := New(expiredCloudProvider{}).Handle(context.Background(), "云端历史")
	if err != nil || !strings.Contains(expired[0].Title, "需要登录") || !strings.Contains(expired[0].Text, "-cookie-file") {
		t.Fatalf("expired=%+v err=%v", expired, err)
	}
	bookshelf, err := New(fakeAuthenticatedProvider{}).Handle(context.Background(), "/bookshelf")
	if err != nil || bookshelf[0].Kind != KindSearch || !strings.Contains(bookshelf[0].Title, "1 本") || !strings.Contains(bookshelf[0].Text, "同步") {
		t.Fatalf("bookshelf=%+v err=%v", bookshelf, err)
	}
}

func TestOfficialFavoriteAndReviewCommands(t *testing.T) {
	state := New(fakeAuthenticatedProvider{})
	state.Book = &fanqie.Book{ID: "book-1", Title: "示例书"}
	replies, err := state.Handle(context.Background(), "收藏")
	if err != nil || !strings.Contains(replies[0].Text, "官网书架") {
		t.Fatalf("favorite=%+v err=%v", replies, err)
	}
	removed, err := state.Handle(context.Background(), "取消收藏")
	if err != nil || !strings.Contains(removed[0].Text, "官网书架移出") {
		t.Fatalf("unfavorite=%+v err=%v", removed, err)
	}
	synced, err := state.Handle(context.Background(), "同步账号")
	if err != nil || len(synced) != 2 || !strings.Contains(synced[0].Text, "官网书架：1 本") || synced[1].Kind != KindSearch || len(state.Books) != 1 {
		t.Fatalf("sync=%+v books=%+v err=%v", synced, state.Books, err)
	}
	reviews, err := state.Handle(context.Background(), "书评")
	if err != nil || len(state.Reviews) != 1 || !strings.Contains(reviews[0].Text, "很好看") {
		t.Fatalf("reviews=%+v err=%v", reviews, err)
	}
	detail, err := state.Handle(context.Background(), "书评 1")
	if err != nil || len(detail) != 3 || detail[0].Book == nil || detail[0].Book.Score != 9.2 || !strings.Contains(detail[1].Text, "评分 10.0") || !strings.Contains(detail[2].Text, "同意") {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	detail, err = state.Handle(context.Background(), "/comment https://fanqienovel.com/comment/7380914885728668734-7437128617000010520?enter_from=search")
	if err != nil || len(detail) != 3 {
		t.Fatalf("comment URL detail=%+v err=%v", detail, err)
	}
}

func TestSyncRequiresLogin(t *testing.T) {
	replies, err := New(fakeProvider{}).Handle(context.Background(), "/sync")
	if err != nil || len(replies) != 1 || !strings.Contains(replies[0].Title, "需要登录") || !strings.Contains(replies[0].Text, "Cookie") {
		t.Fatalf("replies=%+v err=%v", replies, err)
	}
}

func TestCategoryAndAuthorCommands(t *testing.T) {
	state := New(fakeAuthenticatedProvider{})
	categories, err := state.Handle(context.Background(), "分类")
	if err != nil || !strings.Contains(categories[0].Text, "西方奇幻") || !strings.Contains(categories[0].Text, "古风世情") {
		t.Fatalf("categories=%+v err=%v", categories, err)
	}
	ambiguous, err := state.Handle(context.Background(), "分类 科幻末世")
	if err != nil || !strings.Contains(ambiguous[0].Title, "指定") {
		t.Fatalf("ambiguous=%+v err=%v", ambiguous, err)
	}
	ranked, err := state.Handle(context.Background(), "分类 女 古风世情")
	if err != nil || ranked[0].Kind != KindSearch || state.CategoryID != "1139" || len(state.Books) != 1 {
		t.Fatalf("ranked=%+v state=%+v err=%v", ranked, state, err)
	}
	more, err := state.Handle(context.Background(), "更多结果")
	if err != nil || more[0].Kind != KindSearch || !strings.Contains(more[0].Title, "古风世情") {
		t.Fatalf("more=%+v err=%v", more, err)
	}

	authors, err := state.Handle(context.Background(), "热门作者")
	if err != nil || !strings.Contains(authors[0].Text, "空留") || len(state.Authors) != 1 {
		t.Fatalf("authors=%+v err=%v", authors, err)
	}
	works, err := state.Handle(context.Background(), "作者 1")
	if err != nil || works[0].Kind != KindSearch || !strings.Contains(works[0].Title, "空留") || state.Books[0].Title != "作者作品" {
		t.Fatalf("works=%+v state=%+v err=%v", works, state, err)
	}
}

func TestCommandsWithMissingArgumentsShowGuidance(t *testing.T) {
	state := New(fakeProvider{})
	for _, input := range []string{"搜索", "/search", "打开", "/open", "阅读", "/read"} {
		replies, err := state.Handle(context.Background(), input)
		if err != nil || len(replies) != 1 || replies[0].Kind != KindText {
			t.Fatalf("%q: replies=%+v err=%v", input, replies, err)
		}
	}
}

func TestLockedChapterDoesNotAdvanceProgress(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()
	_, _ = state.Handle(ctx, "示例")
	_, _ = state.Handle(ctx, "打开 1")
	replies, err := state.Handle(ctx, "第 3 章")
	if err != nil || replies[0].Kind != KindText || state.Index != -1 {
		t.Fatalf("replies=%+v index=%d err=%v", replies, state.Index, err)
	}
}

func TestChapterLockedAtFetchIsHandledAsUnavailable(t *testing.T) {
	state := New(runtimeLockedProvider{})
	ctx := context.Background()
	_, _ = state.Handle(ctx, "示例")
	_, _ = state.Handle(ctx, "打开 1")
	replies, err := state.Handle(ctx, "第 1 章")
	if err != nil || replies[0].Kind != KindText || state.Index != -1 {
		t.Fatalf("replies=%+v index=%d err=%v", replies, state.Index, err)
	}
}

func TestHelpStatusAndClear(t *testing.T) {
	state := New(fakeProvider{})
	ctx := context.Background()
	for _, input := range []string{"帮助", "/status", "/clear"} {
		replies, err := state.Handle(ctx, input)
		if err != nil || len(replies) != 1 {
			t.Fatalf("%s: replies=%+v err=%v", input, replies, err)
		}
	}
}
