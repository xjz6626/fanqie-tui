package fanqie

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type endlessReader struct{}

func (endlessReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func statePage(state string) string {
	return `<script>window.__INITIAL_STATE__=` + state + `;</script>`
}

func testProvider(t *testing.T) *WebProvider {
	t.Helper()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/novel/channel/homepage/search/search/v1/":
			return response(200, `{"code":0,"data":{"ret_data":[{"book_id":"book-1","title":"示例书","author":"作者","category":"玄幻","creation_status":"0","score":"9.1"}],"offset":10,"has_more":"1"}}`), nil
		case "/api/author/misc/top_book_list/v1/":
			return response(200, `{"code":0,"book_list":[{"book_id":"popular-1","book_name":"热门书","author":"热门作者","category":"都市高武","creation_status":0,"thumb_url":"https://img.example/popular.jpg"}]}`), nil
		case "/api/rank/recommend/list":
			return response(200, `{"code":0,"data":{"total":2,"list":[{"bookId":"recommended-1","bookName":"推荐书","author":"推荐作者","abstract":"推荐简介","category":"科幻末世","thumbUri":"https://img.example/recommended.jpg"}]}}`), nil
		case "/api/node/publication/list":
			return response(200, `{"code":0,"data":{"total_count":11,"publication_list":[{"book_id":"published-1","book_name":"出版书","author":"出版作者","book_type":"青春校园","publisher":"示例出版社","progress":"已出版","thumb_url":"https://img.example/published.jpg"}]}}`), nil
		case "/api/rank/recent/update/list":
			return response(200, `{"code":0,"data":{"total":21,"data":[{"bookId":"recent-1","bookName":"更新书","author":"更新作者","category":"都市","title":"第12章 新篇","needPay":0}]}}`), nil
		case "/api/reader/book/progress":
			return response(200, `{"code":0,"data":[{"book_id":"book-1","item_id":"chapter-2","index":"2","read_progress":"37.5","read_timestamp":"1723456789"},{"item_id":"missing-book"}]}`), nil
		case "/api/reader/book/read_item_list":
			if got := request.URL.Query().Get("book_id"); got != "book-1" {
				t.Fatalf("book_id=%q", got)
			}
			return response(200, `{"code":0,"data":{"read_item_list":[{"itemId":"chapter-1","chapterOrder":1,"progress":100,"updateTime":1723456789000},{"chapter_id":"chapter-2","index":"2","read_progress":"12.5","read_timestamp":"1723456790"}]}}`), nil
		case "/reading/bookapi/bookshelf/info/v:version/":
			if request.Method != http.MethodGet || request.URL.Query().Get("aid") != "1967" || request.Header.Get("Cookie") == "" {
				t.Fatalf("invalid bookshelf request: method=%s query=%s cookie=%t", request.Method, request.URL.RawQuery, request.Header.Get("Cookie") != "")
			}
			return response(200, `{"code":0,"data":{"book_shelf_info":[{"book_id":"shelf-1"},{"book_id":"shelf-2"}]},"message":"SUCCESS"}`), nil
		case "/api/book/simple/info":
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Cookie") == "" {
				t.Fatalf("invalid simple info request: method=%s content-type=%s cookie=%t", request.Method, request.Header.Get("Content-Type"), request.Header.Get("Cookie") != "")
			}
			body, err := io.ReadAll(request.Body)
			if err != nil || !strings.Contains(string(body), `"shelf-1"`) {
				t.Fatalf("simple info body=%q err=%v", body, err)
			}
			return response(200, `{"code":0,"data":{"bookList":[{"book_id":"shelf-1","book_name":"书架一","genre":"都市"},{"book_id":"shelf-2","book_name":"书架二"}]}}`), nil
		case "/api/config/list":
			return response(200, `{"code":0,"data":{"list":[{"id":"1141","name":"西方奇幻","group":["male"]},{"id":"1139","name":"古风世情","group":["female"]},{"id":"8","name":"科幻末世","group":["male","female"]}]}}`), nil
		case "/rank/1_1":
			return response(200, `<script>window.__INITIAL_STATE__={"rank":{"rankVersion":"1786544400"}}</script>`), nil
		case "/api/rank/category/list":
			if request.URL.Query().Get("rank_version") != "1786544400" || request.URL.Query().Get("category_id") == "" {
				t.Fatalf("category query=%s", request.URL.RawQuery)
			}
			return response(200, `{"code":0,"data":{"book_list":[{"bookId":"rank-1","bookName":"分类书","author":"分类作者"}],"total_num":21}}`), nil
		case "/api/author/misc/top_author_list/v1/":
			return response(200, `{"author_list":[{"author":"空留","author_id":"author-1","author_level":"殿堂作家","introduction":"代表作"}]}`), nil
		case "/api/writer/info":
			return response(200, `{"code":0,"data":{"name":"空留","description":"作者简介","fans_count":"45.5","fans_count_unit":"万","word_count":"214","word_count_unit":"万","creation_days":"1191"}}`), nil
		case "/api/writer/book_list":
			return response(200, `{"code":0,"data":{"book_list":[{"book_id":"author-book-1","book_name":"作者作品","author":"空留","category":"古言"}],"total_count":1}}`), nil
		case "/page/book-1":
			return response(200, statePage(`{"page":{"bookId":"book-1","bookName":"示例书","author":"作者","abstract":"简介","thumbUri":"https://img.example/cover.jpg","categoryV2":"[{\"Name\":\"玄幻\"},{\"Name\":\"穿越\"}]","wordNumber":"12345","chapterTotal":"2","creationStatus":1}}`)), nil
		case "/api/reader/directory/detail":
			return response(200, `{"code":0,"data":{"chapterListWithVolume":[[{"itemId":"chapter-1","title":"第一章","realChapterOrder":"1","isChapterLock":"0","needPay":"0","volume_name":"第一卷"}],{"volumeName":"第二卷","chapterList":[{"itemId":"chapter-2","title":"第二章","isChapterLock":"1"}]}]}}`), nil
		case "/reader/chapter-1":
			return response(200, statePage(`{"reader":{"chapterData":{"bookId":"book-1","itemId":"chapter-1","title":"第一章","content":"<p>正文\ue3e8</p><p>第二段</p>","realChapterOrder":"1","nextItemId":"chapter-2","isChapterLock":"0","needPay":0}}}`)), nil
		case "/reader/locked":
			return response(200, statePage(`{"reader":{"chapterData":{"itemId":"locked","content":"不可读","isChapterLock":true}}}`)), nil
		default:
			return response(404, ""), nil
		}
	})
	return NewWebProviderWithClient(&http.Client{Transport: transport})
}

func TestWebProviderSearch(t *testing.T) {
	page, err := testProvider(t).Search(context.Background(), "示例", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Books) != 1 || page.Books[0].Status != "连载" || !page.HasMore || page.NextOffset != 10 {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestWebProviderSearchSkipsUnusableRowsAndAdvancesOffset(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, `{"code":0,"data":{"ret_data":[{"book_id":"","title":"缺少 ID"},{"book_id":"book-1","title":"示例书"}],"offset":0,"has_more":true}}`), nil
	})}
	page, err := NewWebProviderWithClient(client).Search(context.Background(), "示例", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Books) != 1 || page.Books[0].ID != "book-1" || page.NextOffset != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestWebProviderDiscoverFeeds(t *testing.T) {
	provider := testProvider(t)
	tests := []struct {
		name       string
		kind       DiscoverKind
		wantTitle  string
		wantStatus string
		wantMore   bool
		wantNext   int
	}{
		{name: "popular", kind: DiscoverPopular, wantTitle: "热门书", wantStatus: "连载", wantNext: 1},
		{name: "recommended", kind: DiscoverRecommended, wantTitle: "推荐书", wantMore: true, wantNext: 1},
		{name: "published", kind: DiscoverPublished, wantTitle: "出版书", wantStatus: "已出版 · 示例出版社", wantMore: true, wantNext: 10},
		{name: "recent", kind: DiscoverRecent, wantTitle: "更新书", wantStatus: "第12章 新篇", wantMore: true, wantNext: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page, err := provider.Discover(context.Background(), test.kind, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Books) != 1 || page.Books[0].Title != test.wantTitle || page.Books[0].Status != test.wantStatus {
				t.Fatalf("unexpected books: %+v", page.Books)
			}
			if page.HasMore != test.wantMore || page.NextOffset != test.wantNext {
				t.Fatalf("unexpected pagination: %+v", page)
			}
		})
	}
}

func TestWebProviderRecommendationSegments(t *testing.T) {
	for kind, wantType := range map[DiscoverKind]string{DiscoverRecommended: "1", DiscoverMale: "2", DiscoverFemale: "3"} {
		t.Run(string(kind), func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if got := request.URL.Query().Get("type"); got != wantType {
					t.Fatalf("type=%q, want %q", got, wantType)
				}
				return response(200, `{"code":0,"data":{"total":1,"list":[{"bookId":"book-1","bookName":"书"}]}}`), nil
			})}
			page, err := NewWebProviderWithClient(client).Discover(context.Background(), kind, 0)
			if err != nil || len(page.Books) != 1 {
				t.Fatalf("page=%+v err=%v", page, err)
			}
		})
	}
}

func TestRecentUpdatesEmptyPageStopsPagination(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, `{"code":0,"data":{"total":100,"data":[]}}`), nil
	})}
	page, err := NewWebProviderWithClient(client).Discover(context.Background(), DiscoverRecent, 20)
	if err != nil || page.HasMore || page.NextOffset != 20 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestWebProviderSessionAccount(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Cookie"); got != "sessionid=secret" {
			t.Fatalf("Cookie=%q", got)
		}
		switch request.URL.Path {
		case "/api/reader/user/info":
			return response(200, `{"code":0,"data":{"user_info":{"userId":"user-1","nickname":"读者","isVip":true}}}`), nil
		default:
			return response(404, ""), nil
		}
	})}
	provider := &WebProvider{client: client, sessionCookie: "sessionid=secret"}
	account, err := provider.GetAccount(context.Background())
	if err != nil || account.ID != "user-1" || account.Name != "读者" || !account.VIP {
		t.Fatalf("account=%+v err=%v", account, err)
	}
}

func TestWebProviderCloudProgress(t *testing.T) {
	provider := testProvider(t)
	provider.SetSession("sessionid=secret")
	progress, err := provider.GetCloudProgress(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].BookID != "book-1" || progress[0].ChapterID != "chapter-2" || progress[0].ChapterOrder != 2 || progress[0].ReadProgress != 37.5 || !progress[0].ProgressKnown {
		t.Fatalf("progress=%+v", progress)
	}
	if got := progress[0].UpdatedAt; got.Unix() != 1723456789 || got.Location() != time.UTC {
		t.Fatalf("UpdatedAt=%v", got)
	}
}

func TestWebProviderReadItems(t *testing.T) {
	provider := testProvider(t)
	provider.SetSession("sessionid=secret")
	items, err := provider.GetReadItems(context.Background(), " book-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ChapterID != "chapter-1" || items[0].ChapterOrder != 1 || items[0].ReadProgress != 100 || !items[0].ProgressKnown {
		t.Fatalf("items=%+v", items)
	}
	if got := items[0].UpdatedAt.Unix(); got != 1723456789 {
		t.Fatalf("millisecond UpdatedAt.Unix()=%d", got)
	}
}

func TestWebProviderBookshelf(t *testing.T) {
	provider := testProvider(t)
	provider.SetSession("sessionid=secret")
	books, err := provider.GetBookshelf(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 || books[0].ID != "shelf-1" || books[0].Title != "书架一" || books[1].Title != "书架二" {
		t.Fatalf("books=%+v", books)
	}
}

func TestWebProviderOfficialBookshelfWriteAndProgress(t *testing.T) {
	inBookshelf := false
	addCalls := 0
	deleteCalls := 0
	progressCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Cookie") != "sessionid=secret" {
			t.Fatalf("authenticated request omitted Cookie: %s", request.URL.Path)
		}
		switch request.URL.Path {
		case "/reading/bookapi/bookshelf/check/v:version/":
			if request.Method != http.MethodGet || request.URL.Query().Get("book_id") != "book-1" || request.URL.Query().Get("aid") != "1967" {
				t.Fatalf("invalid check request: %s %s", request.Method, request.URL.String())
			}
			return response(200, fmt.Sprintf(`{"code":0,"data":%t}`, inBookshelf)), nil
		case "/reading/bookapi/bookshelf/add/v:version":
			addCalls++
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPost || !strings.Contains(string(body), `"book_id":"book-1"`) || !strings.Contains(string(body), `"book_type":7`) {
				t.Fatalf("invalid add request: method=%s body=%s", request.Method, body)
			}
			inBookshelf = true
			return response(200, `{"code":0,"message":"SUCCESS"}`), nil
		case "/reading/bookapi/bookshelf/delete/v:version":
			deleteCalls++
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPost || !strings.Contains(string(body), `"book_id":"book-1"`) || !strings.Contains(string(body), `"book_type":7`) {
				t.Fatalf("invalid delete request: method=%s body=%s", request.Method, body)
			}
			inBookshelf = false
			return response(200, `{"code":0,"message":"SUCCESS"}`), nil
		case "/api/reader/book/update_progress":
			progressCalls++
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `"book_id":"book-1"`) || !strings.Contains(string(body), `"item_id":"chapter-2"`) || !strings.Contains(string(body), `"index":2`) {
				t.Fatalf("invalid progress body=%s", body)
			}
			return response(200, `{"code":0}`), nil
		default:
			return response(404, ""), nil
		}
	})}
	provider := NewWebProviderWithClient(client)
	provider.SetSession("sessionid=secret")
	ctx := context.Background()
	if in, err := provider.InBookshelf(ctx, "book-1"); err != nil || in {
		t.Fatalf("initial InBookshelf=%t err=%v", in, err)
	}
	if err := provider.AddToBookshelf(ctx, Book{ID: "book-1", BookType: 7}); err != nil {
		t.Fatal(err)
	}
	if err := provider.AddToBookshelf(ctx, Book{ID: "book-1", BookType: 7}); err != nil {
		t.Fatal(err)
	}
	if addCalls != 1 {
		t.Fatalf("addCalls=%d, want idempotent single write", addCalls)
	}
	if err := provider.RemoveFromBookshelf(ctx, Book{ID: "book-1", BookType: 7}); err != nil {
		t.Fatal(err)
	}
	if err := provider.RemoveFromBookshelf(ctx, Book{ID: "book-1", BookType: 7}); err != nil {
		t.Fatal(err)
	}
	if deleteCalls != 1 || inBookshelf {
		t.Fatalf("deleteCalls=%d inBookshelf=%t, want one idempotent delete", deleteCalls, inBookshelf)
	}
	if err := provider.UpdateProgress(ctx, "book-1", "chapter-2", 2); err != nil {
		t.Fatal(err)
	}
	if progressCalls != 1 {
		t.Fatalf("progressCalls=%d", progressCalls)
	}
}

func TestWebProviderPublicReviews(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/sitemap-comment":
			return response(200, `<html><script>window._SSR_DATA = {"data":{"loadersData":{"key":{"data":{"links":[{"url":"https://fanqienovel.com/comment/7380914885728668734-7437128617000010520","title":"写得很好_18号公寓"}]}}}}}</script></html>`), nil
		case "/api/comment/get_book_comment":
			if request.URL.Query().Get("bookId") != "7380914885728668734" || request.URL.Query().Get("commentId") != "7437128617000010520" {
				t.Fatalf("invalid comment query=%s", request.URL.RawQuery)
			}
			return response(200, `{"code":0,"data":{"base_resp":{"code":0},"novel":{"book_id":"7380914885728668734","title":"18号公寓","author_name":"作者","score":8.7,"read_count":19492,"word_count":2073771,"finish_status":"已完结"},"comments":[{"info":{"comment_id":"7437128617000010520","text":"完整书评","score":10,"digg_count":38,"reply_count":4,"read_duration":73884,"create_time":1731595731},"user":{"nick_name":"读者"}}],"replies":[{"info":{"comment_id":"7465439183690876000","text":"回复内容","digg_count":17},"user":{"nick_name":"回复者"}}]}}`), nil
		default:
			return response(404, ""), nil
		}
	})}
	provider := NewWebProviderWithClient(client)
	feed, err := provider.GetReviewFeed(context.Background())
	if err != nil || len(feed) != 1 || feed[0].BookID != "7380914885728668734" || feed[0].BookTitle != "18号公寓" {
		t.Fatalf("feed=%+v err=%v", feed, err)
	}
	detail, err := provider.GetComment(context.Background(), feed[0].BookID, feed[0].CommentID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Book.ID != feed[0].BookID || detail.Book.Score != 8.7 || detail.Book.ReadCount != 19492 || len(detail.Reviews) != 1 || len(detail.Replies) != 1 {
		t.Fatalf("detail=%+v", detail)
	}
	if detail.Reviews[0].ID != feed[0].CommentID || !detail.Reviews[0].ScoreKnown || detail.Reviews[0].ReadDuration != 73884*time.Second {
		t.Fatalf("review=%+v", detail.Reviews[0])
	}
}

func TestWebProviderPublicCategoriesAndAuthors(t *testing.T) {
	provider := testProvider(t)
	categories, err := provider.GetCategories(context.Background())
	if err != nil || len(categories) != 4 || categories[0].Name != "西方奇幻" {
		t.Fatalf("categories=%+v err=%v", categories, err)
	}
	page, err := provider.GetCategoryRank(context.Background(), "1139", "female", 0)
	if err != nil || len(page.Books) != 1 || page.Books[0].Title != "分类书" || !page.HasMore || page.NextOffset != 1 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	authors, err := provider.GetTopAuthors(context.Background())
	if err != nil || len(authors) != 1 || authors[0].ID != "author-1" {
		t.Fatalf("authors=%+v err=%v", authors, err)
	}
	profile, books, err := provider.GetAuthor(context.Background(), "author-1")
	if err != nil || profile.Name != "空留" || profile.Followers != "45.5万" || len(books) != 1 || books[0].Title != "作者作品" {
		t.Fatalf("profile=%+v books=%+v err=%v", profile, books, err)
	}
}

func TestWebProviderReadItemsAcceptsChapterIDList(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, `{"code":0,"data":["chapter-1",12345]}`), nil
	})}
	provider := NewWebProviderWithClient(client)
	provider.SetSession("sessionid=secret")
	items, err := provider.GetReadItems(context.Background(), "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ChapterID != "chapter-1" || items[1].ChapterID != "12345" {
		t.Fatalf("items=%+v", items)
	}
}

func TestCloudProgressDoesNotTreatPercentageAsChapterOrder(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, `{"code":0,"data":[{"book_id":"book-1","read_progress":42.5}]}`), nil
	})}
	provider := NewWebProviderWithClient(client)
	provider.SetSession("sessionid=secret")
	progress, err := provider.GetCloudProgress(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].ChapterOrder != 0 || progress[0].ReadProgress != 42.5 || !progress[0].ProgressKnown {
		t.Fatalf("progress=%+v", progress)
	}
}

func TestWebProviderSessionController(t *testing.T) {
	provider := testProvider(t)
	if provider.HasSession() {
		t.Fatal("new provider unexpectedly has a session")
	}
	provider.SetSession("  sessionid=secret  ")
	if !provider.HasSession() {
		t.Fatal("SetSession did not activate session")
	}
	provider.ClearSession()
	if provider.HasSession() {
		t.Fatal("ClearSession did not remove session")
	}
	if _, err := provider.GetAccount(context.Background()); err != ErrLoginRequired {
		t.Fatalf("GetAccount after ClearSession error=%v", err)
	}
}

func TestWebProviderSessionControllerConcurrentUse(t *testing.T) {
	provider := testProvider(t)
	done := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		go func(worker int) {
			defer func() { done <- struct{}{} }()
			for iteration := 0; iteration < 200; iteration++ {
				if (worker+iteration)%3 == 0 {
					provider.ClearSession()
				} else {
					provider.SetSession("sessionid=secret")
				}
				_ = provider.HasSession()
				_ = provider.session()
			}
		}(worker)
	}
	for worker := 0; worker < 8; worker++ {
		<-done
	}
}

func TestWebProviderSessionIsNotSentToSearchHost(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Cookie"); got != "" {
			t.Fatalf("session cookie leaked to %s: %q", request.URL.Hostname(), got)
		}
		return response(200, `{"code":0,"data":{"ret_data":[],"has_more":false}}`), nil
	})}
	provider := &WebProvider{client: client, sessionCookie: "sessionid=secret"}
	if _, err := provider.Search(context.Background(), "书", 0); err != nil {
		t.Fatal(err)
	}
}

func TestWebProviderSessionIsOnlySentToAuthenticatedEndpoints(t *testing.T) {
	seen := map[string]string{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen[request.URL.Path] = request.Header.Get("Cookie")
		switch request.URL.Path {
		case "/page/book-1":
			return response(200, statePage(`{"page":{"bookId":"book-1","bookName":"示例书","author":"作者"}}`)), nil
		case "/api/reader/user/info":
			return response(200, `{"code":0,"data":{"user_id":"user-1","name":"读者"}}`), nil
		default:
			return response(404, ""), nil
		}
	})}
	provider := NewWebProviderWithClient(client)
	provider.SetSession("sessionid=secret")
	if _, err := provider.GetBook(context.Background(), "book-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.GetAccount(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := seen["/page/book-1"]; got != "" {
		t.Fatalf("session was sent to public book page: %q", got)
	}
	if got := seen["/api/reader/user/info"]; got != "sessionid=secret" {
		t.Fatalf("authenticated endpoint cookie=%q", got)
	}
}

func TestWebProviderRedirectStripsSessionOutsideExactHTTPSOrigin(t *testing.T) {
	provider := NewWebProviderWithSession(time.Second, "sessionid=secret")
	for _, address := range []string{"https://example.com/path", "https://sub.fanqienovel.com/path", "http://fanqienovel.com/path", "https://fanqienovel.com:8443/path", "https://fanqienovel.com/page/book-1"} {
		request, err := http.NewRequest(http.MethodGet, address, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Cookie", "sessionid=secret")
		if err := provider.client.CheckRedirect(request, nil); err != nil {
			t.Fatal(err)
		}
		if got := request.Header.Get("Cookie"); got != "" {
			t.Fatalf("Cookie retained for %s: %q", address, got)
		}
	}
	authRequest, _ := http.NewRequest(http.MethodGet, accountURL, nil)
	authRequest.Header.Set("Cookie", "sessionid=secret")
	if err := provider.client.CheckRedirect(authRequest, nil); err != nil {
		t.Fatal(err)
	}
	if got := authRequest.Header.Get("Cookie"); got != "sessionid=secret" {
		t.Fatalf("Cookie stripped from authenticated endpoint: %q", got)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://fanqienovel.com/path", nil)
	request.Header.Set("Cookie", "sessionid=secret")
	via := make([]*http.Request, 10)
	if err := provider.client.CheckRedirect(request, via); err == nil || !strings.Contains(err.Error(), "重定向次数过多") {
		t.Fatalf("redirect limit error=%v", err)
	}
}

func TestWebProviderWithClientInstallsSafeRedirectPolicy(t *testing.T) {
	callerPolicyRan := false
	wantErr := errors.New("caller denied redirect")
	provider := NewWebProviderWithClient(&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		callerPolicyRan = true
		return wantErr
	}})
	request, _ := http.NewRequest(http.MethodGet, "https://sub.fanqienovel.com/path", nil)
	request.Header.Set("Cookie", "sessionid=secret")
	if err := provider.client.CheckRedirect(request, nil); !errors.Is(err, wantErr) {
		t.Fatalf("redirect error=%v", err)
	}
	if !callerPolicyRan || request.Header.Get("Cookie") != "" {
		t.Fatalf("Cookie retained by supplied client: %q", request.Header.Get("Cookie"))
	}
}

func TestWebProviderLoginRequired(t *testing.T) {
	if _, err := testProvider(t).GetAccount(context.Background()); err != ErrLoginRequired {
		t.Fatalf("GetAccount without session error=%v", err)
	}
	if _, err := testProvider(t).GetCloudProgress(context.Background()); err != ErrLoginRequired {
		t.Fatalf("GetCloudProgress without session error=%v", err)
	}
	if _, err := testProvider(t).GetReadItems(context.Background(), "book-1"); err != ErrLoginRequired {
		t.Fatalf("GetReadItems without session error=%v", err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, `{"code":-1,"data":{},"message":"请先登录"}`), nil
	})}
	provider := &WebProvider{client: client, sessionCookie: "expired=1"}
	if _, err := provider.GetAccount(context.Background()); err != ErrLoginRequired {
		t.Fatalf("expired session error=%v", err)
	}
}

func TestWebProviderRecognizesExpiredSessionResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{}`},
		{name: "login html", status: http.StatusOK, body: `<html><a href="/login">登录</a></html>`},
		{name: "msg field", status: http.StatusOK, body: `{"code":-1,"msg":"用户未登录"}`},
		{name: "null account", status: http.StatusOK, body: `{"code":0,"data":null}`},
		{name: "nickname without stable id", status: http.StatusOK, body: `{"code":0,"data":{"nickname":"游客"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(test.status, test.body), nil
			})}
			provider := NewWebProviderWithClient(client)
			provider.SetSession("expired=1")
			if _, err := provider.GetAccount(context.Background()); !errors.Is(err, ErrLoginRequired) {
				t.Fatalf("GetAccount error=%v", err)
			}
		})
	}
}

func TestReadItemsRequiresBookIDAndList(t *testing.T) {
	provider := testProvider(t)
	provider.SetSession("sessionid=secret")
	if _, err := provider.GetReadItems(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "book ID") {
		t.Fatalf("empty book ID error=%v", err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(200, `{"code":0,"data":{"unexpected":true}}`), nil
	})}
	provider = &WebProvider{client: client, sessionCookie: "sessionid=secret"}
	if _, err := provider.GetReadItems(context.Background(), "book-1"); err == nil || !strings.Contains(err.Error(), "data 列表") {
		t.Fatalf("unexpected payload error=%v", err)
	}
}

func TestSessionCapabilityContracts(t *testing.T) {
	var provider any = testProvider(t)
	if _, ok := provider.(SessionController); !ok {
		t.Fatal("WebProvider does not implement SessionController")
	}
	if _, ok := provider.(CloudProgressProvider); !ok {
		t.Fatal("WebProvider does not implement CloudProgressProvider")
	}
	if _, ok := provider.(ReadItemsProvider); !ok {
		t.Fatal("WebProvider does not implement ReadItemsProvider")
	}
	if _, ok := provider.(BookshelfProvider); !ok {
		t.Fatal("WebProvider does not implement BookshelfProvider")
	}
	if _, ok := provider.(BookshelfController); !ok {
		t.Fatal("WebProvider does not implement BookshelfController")
	}
	if _, ok := provider.(CategoryProvider); !ok {
		t.Fatal("WebProvider does not implement CategoryProvider")
	}
	if _, ok := provider.(AuthorProvider); !ok {
		t.Fatal("WebProvider does not implement AuthorProvider")
	}
}

func TestWebProviderRejectsUnknownDiscoveryFeed(t *testing.T) {
	_, err := testProvider(t).Discover(context.Background(), DiscoverKind("unknown"), 0)
	if err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebProviderBookAndDirectory(t *testing.T) {
	provider := testProvider(t)
	book, err := provider.GetBook(context.Background(), "book-1")
	if err != nil {
		t.Fatal(err)
	}
	if book.Category != "玄幻 / 穿越" || book.Status != "完结" || book.WordCount != 12345 {
		t.Fatalf("unexpected book: %+v", book)
	}
	chapters, err := provider.GetDirectory(context.Background(), book.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Locked || !chapters[1].Locked || chapters[1].Volume != "第二卷" {
		t.Fatalf("unexpected chapters: %+v", chapters)
	}
}

func TestWebProviderChapter(t *testing.T) {
	chapter, err := testProvider(t).GetChapter(context.Background(), "chapter-1")
	if err != nil {
		t.Fatal(err)
	}
	if chapter.Content != "正文D\n\n第二段" || chapter.NextID != "chapter-2" {
		t.Fatalf("unexpected chapter: %+v", chapter)
	}
}

func TestWebProviderLockedAndNotFound(t *testing.T) {
	provider := testProvider(t)
	if _, err := provider.GetChapter(context.Background(), "locked"); err != ErrLocked {
		t.Fatalf("got %v, want ErrLocked", err)
	}
	if _, err := provider.GetBook(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestWebProviderNetworkError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("offline")
	})}
	_, err := NewWebProviderWithClient(client).Search(context.Background(), "书", 0)
	if err == nil || !strings.Contains(err.Error(), "无法连接") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebProviderRejectsOversizedResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(endlessReader{}),
		}, nil
	})}
	_, err := NewWebProviderWithClient(client).Search(context.Background(), "书", 0)
	if err == nil || !strings.Contains(err.Error(), "响应过大") {
		t.Fatalf("unexpected error: %v", err)
	}
}
