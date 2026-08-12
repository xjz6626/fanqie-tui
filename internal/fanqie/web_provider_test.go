package fanqie

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
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
