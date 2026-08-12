package fanqie

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	searchURL    = "https://novel.snssdk.com/api/novel/channel/homepage/search/search/v1/"
	bookURL      = "https://fanqienovel.com/page/"
	directoryURL = "https://fanqienovel.com/api/reader/directory/detail"
	chapterURL   = "https://fanqienovel.com/reader/"
)

// WebProvider reads unlocked content from Fanqie's public website.
type WebProvider struct {
	client *http.Client
}

// NewWebProvider creates a public web provider with a bounded timeout.
func NewWebProvider(timeout time.Duration) *WebProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 6
	transport.ResponseHeaderTimeout = timeout
	return &WebProvider{client: &http.Client{Timeout: timeout, Transport: transport}}
}

// NewWebProviderWithClient creates a provider around a supplied client.
// It is primarily useful for tests and alternate transports.
func NewWebProviderWithClient(client *http.Client) *WebProvider {
	return &WebProvider{client: client}
}

func (provider *WebProvider) request(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("无法连接番茄小说：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("上游返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("读取上游响应失败：%w", err)
	}
	return body, nil
}

func (provider *WebProvider) json(ctx context.Context, address string) (map[string]any, error) {
	body, err := provider.request(ctx, address)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, &ParseError{Message: "上游没有返回 JSON"}
	}
	code := asInt(payload["code"], 0)
	if code != 0 && code != 200 {
		return nil, &ParseError{Message: fmt.Sprintf("上游接口错误 %d: %s", code, asString(payload["message"]))}
	}
	return payload, nil
}

func (provider *WebProvider) page(ctx context.Context, address string) (string, map[string]any, error) {
	body, err := provider.request(ctx, address)
	if err != nil {
		return "", nil, err
	}
	page := string(body)
	state, err := extractInitialState(page)
	return page, state, err
}

// Search finds public books matching query.
func (provider *WebProvider) Search(ctx context.Context, query string, offset int) (SearchPage, error) {
	if strings.TrimSpace(query) == "" {
		return SearchPage{}, nil
	}
	parameters := url.Values{}
	parameters.Set("q", query)
	parameters.Set("aid", "1967")
	parameters.Set("offset", strconv.Itoa(offset))
	payload, err := provider.json(ctx, searchURL+"?"+parameters.Encode())
	if err != nil {
		return SearchPage{}, err
	}
	data := object(payload["data"])
	if data == nil {
		return SearchPage{}, &ParseError{Message: "搜索结果缺少 data"}
	}
	result := SearchPage{NextOffset: asInt(data["offset"], offset), HasMore: asBool(data["has_more"])}
	rows, _ := data["ret_data"].([]any)
	for _, value := range rows {
		row := object(value)
		if row == nil {
			continue
		}
		result.Books = append(result.Books, Book{
			ID:       asString(first(row, "book_id", "bookId")),
			Title:    asString(first(row, "title", "book_name", "bookName")),
			Author:   asString(first(row, "author", "author_name")),
			Abstract: asString(first(row, "abstract", "book_abstract")),
			CoverURL: asString(first(row, "thumb_url", "cover")),
			Category: asString(row["category"]),
			Score:    asFloat(row["score"]),
			Status:   creationStatus(row["creation_status"]),
		})
	}
	if data["offset"] == nil {
		result.NextOffset = offset + len(result.Books)
	}
	return result, nil
}

// GetBook fetches public book metadata.
func (provider *WebProvider) GetBook(ctx context.Context, bookID string) (Book, error) {
	_, state, err := provider.page(ctx, bookURL+url.PathEscape(bookID))
	if err != nil {
		return Book{}, err
	}
	page := object(state["page"])
	if page == nil || asString(first(page, "bookId", "book_id")) == "" {
		return Book{}, &ParseError{Message: "书籍页面缺少书籍信息"}
	}
	return Book{
		ID:           asString(first(page, "bookId", "book_id")),
		Title:        decrypt(asString(first(page, "bookName", "book_name"))),
		Author:       decrypt(asString(first(page, "authorName", "author", "author_name"))),
		Abstract:     decrypt(asString(first(page, "abstract", "description"))),
		CoverURL:     asString(first(page, "thumbUrl", "thumbUri", "thumb_url")),
		Category:     category(page),
		WordCount:    asInt(first(page, "wordNumber", "word_count"), 0),
		ChapterCount: asInt(first(page, "chapterTotal", "chapter_count"), 0),
		Status:       creationStatus(first(page, "creationStatus", "creation_status")),
	}, nil
}

// GetDirectory fetches all chapter entries for a book.
func (provider *WebProvider) GetDirectory(ctx context.Context, bookID string) ([]Chapter, error) {
	parameters := url.Values{"bookId": []string{bookID}}
	payload, err := provider.json(ctx, directoryURL+"?"+parameters.Encode())
	if err != nil {
		return nil, err
	}
	data := object(payload["data"])
	if data == nil {
		return nil, &ParseError{Message: "目录结果缺少 data"}
	}
	volumes, _ := first(data, "chapterListWithVolume", "chapter_list_with_volume").([]any)
	chapters := make([]Chapter, 0)
	for _, volumeValue := range volumes {
		volumeName := ""
		rows, isRows := volumeValue.([]any)
		if !isRows {
			volume := object(volumeValue)
			if volume == nil {
				continue
			}
			volumeName = asString(first(volume, "volumeName", "volume_name", "title"))
			rows, _ = first(volume, "chapterList", "chapter_list", "chapters").([]any)
		}
		for _, rowValue := range rows {
			row := object(rowValue)
			if row == nil {
				continue
			}
			fallbackOrder := len(chapters) + 1
			rowVolume := asString(first(row, "volume_name", "volumeName"))
			if rowVolume == "" {
				rowVolume = volumeName
			}
			chapters = append(chapters, Chapter{
				ID:      asString(first(row, "itemId", "item_id")),
				Title:   asString(first(row, "title", "chapter_title")),
				Order:   asInt(first(row, "realChapterOrder", "order"), fallbackOrder),
				Volume:  rowVolume,
				Locked:  asBool(first(row, "isChapterLock", "is_chapter_lock")),
				NeedPay: asBool(first(row, "needPay", "need_pay")),
			})
		}
	}
	if len(chapters) == 0 {
		return nil, &ParseError{Message: "没有解析到章节，书籍可能已下架或网页结构已经变化"}
	}
	return chapters, nil
}

// GetChapter fetches and decodes one unlocked chapter.
func (provider *WebProvider) GetChapter(ctx context.Context, itemID string) (ChapterContent, error) {
	_, state, err := provider.page(ctx, chapterURL+url.PathEscape(itemID))
	if err != nil {
		return ChapterContent{}, err
	}
	reader := object(state["reader"])
	data := object(reader["chapterData"])
	if data == nil || len(data) == 0 {
		return ChapterContent{}, &ParseError{Message: "章节页面缺少 chapterData"}
	}
	locked := asBool(first(data, "isChapterLock", "is_chapter_lock"))
	needPay := asBool(first(data, "needPay", "need_pay"))
	if locked || needPay {
		return ChapterContent{}, ErrLocked
	}
	rawContent := asString(data["content"])
	if rawContent == "" {
		return ChapterContent{}, &ParseError{Message: "章节正文为空，可能需要登录、验证或已被下架"}
	}
	return ChapterContent{
		BookID:     asString(first(data, "bookId", "book_id")),
		ChapterID:  asString(first(data, "itemId", "item_id")),
		Title:      decrypt(asString(first(data, "title", "chapterTitle"))),
		Content:    decrypt(htmlToText(rawContent)),
		Order:      asInt(first(data, "realChapterOrder", "order"), 0),
		PreviousID: asString(first(data, "preItemId", "pre_item_id")),
		NextID:     asString(first(data, "nextItemId", "next_item_id")),
		Locked:     locked,
		NeedPay:    needPay,
		BookName:   asString(first(data, "bookName", "book_name")),
	}, nil
}
