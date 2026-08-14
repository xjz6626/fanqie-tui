package fanqie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	searchURL           = "https://novel.snssdk.com/api/novel/channel/homepage/search/search/v1/"
	popularURL          = "https://fanqienovel.com/api/author/misc/top_book_list/v1/"
	recommendedURL      = "https://fanqienovel.com/api/rank/recommend/list"
	publishedURL        = "https://fanqienovel.com/api/node/publication/list"
	recentUpdatesURL    = "https://fanqienovel.com/api/rank/recent/update/list"
	accountURL          = "https://fanqienovel.com/api/reader/user/info"
	cloudProgressURL    = "https://fanqienovel.com/api/reader/book/progress"
	readItemsURL        = "https://fanqienovel.com/api/reader/book/read_item_list"
	updateProgressURL   = "https://fanqienovel.com/api/reader/book/update_progress"
	bookshelfURL        = "https://fanqienovel.com/reading/bookapi/bookshelf/info/v:version/"
	bookshelfCheckURL   = "https://fanqienovel.com/reading/bookapi/bookshelf/check/v:version/"
	bookshelfAddURL     = "https://fanqienovel.com/reading/bookapi/bookshelf/add/v:version"
	bookshelfDeleteURL  = "https://fanqienovel.com/reading/bookapi/bookshelf/delete/v:version"
	simpleBookInfoURL   = "https://fanqienovel.com/api/book/simple/info"
	commentSitemapURL   = "https://fanqienovel.com/sitemap-comment"
	commentDetailURL    = "https://fanqienovel.com/api/comment/get_book_comment"
	categoryConfigURL   = "https://fanqienovel.com/api/config/list"
	categoryRankURL     = "https://fanqienovel.com/api/rank/category/list"
	rankPageURL         = "https://fanqienovel.com/rank/1_1"
	topAuthorsURL       = "https://fanqienovel.com/api/author/misc/top_author_list/v1/"
	writerInfoURL       = "https://fanqienovel.com/api/writer/info"
	writerBooksURL      = "https://fanqienovel.com/api/writer/book_list"
	bookURL             = "https://fanqienovel.com/page/"
	directoryURL        = "https://fanqienovel.com/api/reader/directory/detail"
	chapterURL          = "https://fanqienovel.com/reader/"
	maxResponseBodySize = 16 << 20
)

var rankVersionPattern = regexp.MustCompile(`rankVersion\\?"\s*:\s*\\?"(\d+)`)
var commentURLPattern = regexp.MustCompile(`^https://fanqienovel\.com/comment/(\d+)-(\d+)$`)

// WebProvider reads unlocked content from Fanqie's public website.
type WebProvider struct {
	client        *http.Client
	sessionMu     sync.RWMutex
	sessionCookie string
}

// NewWebProvider creates a public web provider with a bounded timeout.
func NewWebProvider(timeout time.Duration) *WebProvider {
	return newWebProvider(timeout, "")
}

// NewWebProviderWithSession creates a provider that can use official account
// endpoints. The cookie is supplied by the caller and is never persisted here.
func NewWebProviderWithSession(timeout time.Duration, cookie string) *WebProvider {
	return newWebProvider(timeout, strings.TrimSpace(cookie))
}

func newWebProvider(timeout time.Duration, cookie string) *WebProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 6
	transport.ResponseHeaderTimeout = timeout
	transport.MaxResponseHeaderBytes = 1 << 20
	client := &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: safeRedirect}
	return &WebProvider{client: client, sessionCookie: cookie}
}

// NewWebProviderWithClient creates a provider around a supplied client.
// It is primarily useful for tests and alternate transports.
func NewWebProviderWithClient(client *http.Client) *WebProvider {
	if client == nil {
		client = &http.Client{}
	}
	clone := *client
	callerRedirect := client.CheckRedirect
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := safeRedirect(request, via); err != nil {
			return err
		}
		if callerRedirect != nil {
			if err := callerRedirect(request, via); err != nil {
				request.Header.Del("Cookie")
				return err
			}
		}
		if !isOfficialOrigin(request.URL) {
			request.Header.Del("Cookie")
		}
		return nil
	}
	client = &clone
	return &WebProvider{client: client}
}

func safeRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("上游重定向次数过多")
	}
	if !isAuthenticatedEndpoint(request.URL) {
		request.Header.Del("Cookie")
	}
	return nil
}

// HasSession reports whether an imported browser session is currently active.
func (provider *WebProvider) HasSession() bool {
	provider.sessionMu.RLock()
	defer provider.sessionMu.RUnlock()
	return provider.sessionCookie != ""
}

// SetSession replaces the in-memory browser session. It does not persist it.
func (provider *WebProvider) SetSession(cookie string) {
	provider.sessionMu.Lock()
	provider.sessionCookie = strings.TrimSpace(cookie)
	provider.sessionMu.Unlock()
}

// ClearSession immediately disables authenticated requests in this process.
func (provider *WebProvider) ClearSession() {
	provider.SetSession("")
}

func (provider *WebProvider) session() string {
	provider.sessionMu.RLock()
	defer provider.sessionMu.RUnlock()
	return provider.sessionCookie
}

func (provider *WebProvider) request(ctx context.Context, address string) ([]byte, error) {
	return provider.requestWithSession(ctx, address, false)
}

func (provider *WebProvider) requestWithSession(ctx context.Context, address string, authenticated bool) ([]byte, error) {
	return provider.requestMethod(ctx, http.MethodGet, address, authenticated, nil)
}

func (provider *WebProvider) requestMethod(ctx context.Context, method, address string, authenticated bool, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		if cookie := provider.session(); cookie != "" && isAuthenticatedEndpoint(request.URL) {
			request.Header.Set("Cookie", cookie)
		}
	}
	response, err := provider.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("无法连接番茄小说：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if authenticated && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
		return nil, ErrLoginRequired
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("上游返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxResponseBodySize {
		return nil, &ParseError{Message: "上游响应过大，已停止读取"}
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("读取上游响应失败：%w", err)
	}
	if len(responseBody) > maxResponseBodySize {
		return nil, &ParseError{Message: "上游响应过大，已停止读取"}
	}
	return responseBody, nil
}

func isOfficialOrigin(address *url.URL) bool {
	return address != nil && address.Scheme == "https" &&
		(address.Host == "fanqienovel.com" || address.Host == "fanqienovel.com:443")
}

func isAuthenticatedEndpoint(address *url.URL) bool {
	if !isOfficialOrigin(address) {
		return false
	}
	switch address.Path {
	case "/api/reader/user/info", "/api/reader/book/progress", "/api/reader/book/read_item_list",
		"/api/reader/book/update_progress",
		"/reading/bookapi/bookshelf/info/v:version/", "/reading/bookapi/bookshelf/check/v:version/",
		"/reading/bookapi/bookshelf/add/v:version", "/reading/bookapi/bookshelf/delete/v:version",
		"/api/book/simple/info":
		return true
	default:
		return false
	}
}

func (provider *WebProvider) json(ctx context.Context, address string) (map[string]any, error) {
	return provider.jsonRequest(ctx, address, false)
}

func (provider *WebProvider) authenticatedJSON(ctx context.Context, address string) (map[string]any, error) {
	return provider.jsonRequest(ctx, address, true)
}

func (provider *WebProvider) authenticatedJSONPost(ctx context.Context, address string, value any) (map[string]any, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("编码官网请求失败：%w", err)
	}
	response, err := provider.requestMethod(ctx, http.MethodPost, address, true, body)
	if err != nil {
		return nil, err
	}
	return decodeJSONResponse(response, true)
}

func bookshelfParameters() url.Values {
	return url.Values{
		"aid":                 {"1967"},
		"iid":                 {"0"},
		"version_code":        {"0"},
		"update_version_code": {"0"},
	}
}

func (provider *WebProvider) jsonRequest(ctx context.Context, address string, authenticated bool) (map[string]any, error) {
	body, err := provider.requestWithSession(ctx, address, authenticated)
	if err != nil {
		return nil, err
	}
	return decodeJSONResponse(body, authenticated)
}

func decodeJSONResponse(body []byte, authenticated bool) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		if authenticated && isLoginMessage(string(body)) {
			return nil, ErrLoginRequired
		}
		return nil, &ParseError{Message: "上游没有返回 JSON"}
	}
	code := asInt(payload["code"], 0)
	if code != 0 && code != 200 {
		message := asString(first(payload, "message", "msg"))
		if isLoginMessage(message) {
			return nil, ErrLoginRequired
		}
		return nil, &ParseError{Message: fmt.Sprintf("上游接口错误 %d: %s", code, message)}
	}
	return payload, nil
}

func isLoginMessage(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(normalized, "login") || strings.Contains(normalized, "登录") || strings.Contains(normalized, "/login")
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
		if book, ok := bookFromRow(row); ok {
			result.Books = append(result.Books, book)
		}
	}
	if data["offset"] == nil {
		result.NextOffset = offset + len(result.Books)
	} else if result.HasMore && result.NextOffset <= offset {
		result.NextOffset = offset + len(result.Books)
	}
	return result, nil
}

// Discover fetches a public book-discovery feed without requiring login.
func (provider *WebProvider) Discover(ctx context.Context, kind DiscoverKind, offset int) (SearchPage, error) {
	offset = max(0, offset)
	switch kind {
	case DiscoverPopular:
		parameters := url.Values{"limit": {"30"}, "offset": {"0"}}
		payload, err := provider.json(ctx, popularURL+"?"+parameters.Encode())
		if err != nil {
			return SearchPage{}, err
		}
		return booksPage(payload["book_list"], offset, false, 0), nil
	case DiscoverRecommended, DiscoverMale, DiscoverFemale:
		recommendType := "1"
		if kind == DiscoverMale {
			recommendType = "2"
		} else if kind == DiscoverFemale {
			recommendType = "3"
		}
		parameters := url.Values{"type": {recommendType}, "limit": {"10"}, "offset": {strconv.Itoa(offset)}}
		payload, err := provider.json(ctx, recommendedURL+"?"+parameters.Encode())
		if err != nil {
			return SearchPage{}, err
		}
		data := object(payload["data"])
		if data == nil {
			return SearchPage{}, &ParseError{Message: "推荐榜结果缺少 data"}
		}
		total := asInt(data["total"], 0)
		return booksPage(data["list"], offset, total > offset, total), nil
	case DiscoverPublished:
		const pageSize = 10
		parameters := url.Values{
			"page_index": {strconv.Itoa(offset / pageSize)},
			"page_count": {strconv.Itoa(pageSize)},
		}
		payload, err := provider.json(ctx, publishedURL+"?"+parameters.Encode())
		if err != nil {
			return SearchPage{}, err
		}
		data := object(payload["data"])
		if data == nil {
			return SearchPage{}, &ParseError{Message: "出版榜结果缺少 data"}
		}
		total := asInt(data["total_count"], 0)
		page := booksPage(data["publication_list"], offset, total > offset, total)
		page.NextOffset = (offset/pageSize + 1) * pageSize
		page.HasMore = page.NextOffset < total
		return page, nil
	case DiscoverRecent:
		const pageSize = 20
		parameters := url.Values{"offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(pageSize)}}
		payload, err := provider.json(ctx, recentUpdatesURL+"?"+parameters.Encode())
		if err != nil {
			return SearchPage{}, err
		}
		data := object(payload["data"])
		if data == nil {
			return SearchPage{}, &ParseError{Message: "最近更新结果缺少 data"}
		}
		total := asInt(data["total"], 0)
		rows, _ := data["data"].([]any)
		page := SearchPage{NextOffset: offset + len(rows), HasMore: offset+len(rows) < total}
		if len(rows) == 0 {
			page.HasMore = false
		}
		for _, value := range rows {
			row := object(value)
			if row == nil {
				continue
			}
			book, ok := bookFromRow(row)
			if !ok {
				continue
			}
			payment := ""
			if asBool(row["needPay"]) {
				payment = "需付费"
			}
			book.Status = strings.Join(nonEmptyStrings(decrypt(asString(row["title"])), payment), " · ")
			page.Books = append(page.Books, book)
		}
		return page, nil
	default:
		return SearchPage{}, &ParseError{Message: "不支持的书籍榜单"}
	}
}

// GetAccount checks the account associated with the imported web session.
func (provider *WebProvider) GetAccount(ctx context.Context) (Account, error) {
	if !provider.HasSession() {
		return Account{}, ErrLoginRequired
	}
	payload, err := provider.authenticatedJSON(ctx, accountURL)
	if err != nil {
		return Account{}, err
	}
	data := object(payload["data"])
	if data == nil {
		return Account{}, ErrLoginRequired
	}
	if nested := object(first(data, "user_info", "userInfo", "user")); nested != nil {
		data = nested
	}
	vipValue, vipKnown := firstPresent(data, "is_vip", "isVip", "vip")
	account := Account{
		ID:        asString(first(data, "user_id", "userId", "id", "uid")),
		Name:      decrypt(asString(first(data, "username", "name", "nickname", "screen_name"))),
		AvatarURL: asString(first(data, "avatar", "avatar_url", "avatarUrl")),
		VIP:       asBool(vipValue),
		VIPKnown:  vipKnown,
	}
	if account.ID == "" || account.ID == "0" {
		return Account{}, ErrLoginRequired
	}
	return account, nil
}

// GetCloudProgress returns cloud reading positions without changing them.
func (provider *WebProvider) GetCloudProgress(ctx context.Context) ([]CloudProgress, error) {
	if !provider.HasSession() {
		return nil, ErrLoginRequired
	}
	payload, err := provider.authenticatedJSON(ctx, cloudProgressURL)
	if err != nil {
		return nil, err
	}
	rows, ok := rowsFromData(payload["data"])
	if !ok {
		return nil, &ParseError{Message: "云阅读进度接口缺少 data 列表"}
	}
	progress := make([]CloudProgress, 0, len(rows))
	for _, value := range rows {
		row := object(value)
		if row == nil {
			continue
		}
		bookID := asString(first(row, "book_id", "bookId"))
		if bookID == "" {
			continue
		}
		readProgress, progressKnown := firstPresent(row, "read_progress", "readProgress", "progress")
		progress = append(progress, CloudProgress{
			BookID:        bookID,
			ChapterID:     asString(first(row, "item_id", "itemId", "chapter_id", "chapterId")),
			ChapterOrder:  asInt(first(row, "index", "chapter_order", "chapterOrder"), 0),
			ReadProgress:  asFloat(readProgress),
			ProgressKnown: progressKnown,
			UpdatedAt:     unixTime(first(row, "read_timestamp", "readTimestamp", "update_time", "updateTime")),
		})
	}
	return progress, nil
}

// GetReadItems returns cloud per-chapter reading positions for a book without
// writing reading history or changing the official account.
func (provider *WebProvider) GetReadItems(ctx context.Context, bookID string) ([]ReadItem, error) {
	if !provider.HasSession() {
		return nil, ErrLoginRequired
	}
	bookID = strings.TrimSpace(bookID)
	if bookID == "" {
		return nil, &ParseError{Message: "查询已读章节需要 book ID"}
	}
	parameters := url.Values{"book_id": {bookID}}
	payload, err := provider.authenticatedJSON(ctx, readItemsURL+"?"+parameters.Encode())
	if err != nil {
		return nil, err
	}
	rows, ok := rowsFromData(payload["data"])
	if !ok {
		return nil, &ParseError{Message: "已读章节接口缺少 data 列表"}
	}
	items := make([]ReadItem, 0, len(rows))
	for _, value := range rows {
		row := object(value)
		if row == nil {
			chapterID := strings.TrimSpace(asString(value))
			if chapterID != "" {
				items = append(items, ReadItem{ChapterID: chapterID})
			}
			continue
		}
		chapterID := asString(first(row, "item_id", "itemId", "chapter_id", "chapterId"))
		if chapterID == "" {
			continue
		}
		readProgress, progressKnown := firstPresent(row, "read_progress", "readProgress", "progress")
		items = append(items, ReadItem{
			ChapterID:     chapterID,
			ChapterOrder:  asInt(first(row, "index", "chapter_order", "chapterOrder"), 0),
			ReadProgress:  asFloat(readProgress),
			ProgressKnown: progressKnown,
			UpdatedAt:     unixTime(first(row, "read_timestamp", "readTimestamp", "update_time", "updateTime")),
		})
	}
	return items, nil
}

// GetBookshelf returns the account's official bookshelf without modifying it.
// The website uses a literal v:version route plus common client parameters,
// then a read-only POST to enrich the returned book IDs in one batch.
func (provider *WebProvider) GetBookshelf(ctx context.Context) ([]Book, error) {
	if !provider.HasSession() {
		return nil, ErrLoginRequired
	}
	parameters := bookshelfParameters()
	payload, err := provider.authenticatedJSON(ctx, bookshelfURL+"?"+parameters.Encode())
	if err != nil {
		return nil, err
	}
	data := object(payload["data"])
	if data == nil {
		return nil, &ParseError{Message: "官网书架接口缺少 data"}
	}

	ids := make([]string, 0)
	seenIDs := make(map[string]bool)
	metadata := make(map[string]map[string]any)
	collect := func(value any, recordOrder bool) {
		rows, _ := value.([]any)
		for _, value := range rows {
			row := object(value)
			if row == nil {
				continue
			}
			id := asString(first(row, "book_id", "bookId"))
			if id == "" {
				continue
			}
			if recordOrder && !seenIDs[id] {
				seenIDs[id] = true
				ids = append(ids, id)
			}
			if _, ok := bookFromRow(row); ok {
				metadata[id] = row
			}
		}
	}
	collect(data["book_shelf_info"], true)
	collect(data["book_list"], len(ids) == 0)
	collect(data["book_list_info"], len(ids) == 0)
	if len(ids) == 0 {
		return []Book{}, nil
	}

	bookIDs := make([]string, len(ids))
	copy(bookIDs, ids)
	details, err := provider.authenticatedJSONPost(ctx, simpleBookInfoURL, map[string]any{"book_ids": bookIDs})
	if err != nil {
		return nil, err
	}
	detailData := object(details["data"])
	if detailData == nil {
		return nil, &ParseError{Message: "官网书架书籍信息缺少 data"}
	}
	collect(detailData["bookList"], false)

	books := make([]Book, 0, len(ids))
	for _, id := range ids {
		row := metadata[id]
		book, ok := bookFromRow(row)
		if !ok {
			book = Book{ID: id, Title: "书籍 " + id, Status: "官网书架"}
		} else if book.Status == "" {
			book.Status = "官网书架"
		}
		books = append(books, book)
	}
	return books, nil
}

// InBookshelf checks the account's official bookshelf without changing it.
func (provider *WebProvider) InBookshelf(ctx context.Context, bookID string) (bool, error) {
	if !provider.HasSession() {
		return false, ErrLoginRequired
	}
	bookID = strings.TrimSpace(bookID)
	if bookID == "" {
		return false, &ParseError{Message: "检查官网书架需要 book ID"}
	}
	parameters := bookshelfParameters()
	parameters.Set("book_id", bookID)
	payload, err := provider.authenticatedJSON(ctx, bookshelfCheckURL+"?"+parameters.Encode())
	if err != nil {
		return false, err
	}
	value := payload["data"]
	if row := object(value); row != nil {
		if status, ok := firstPresent(row, "status", "in_bookshelf", "inBookshelf"); ok {
			return asBool(status), nil
		}
	}
	return asBool(value), nil
}

// AddToBookshelf adds a book to the account's official bookshelf. Repeated
// calls are made idempotent by the website and by the preceding status check.
func (provider *WebProvider) AddToBookshelf(ctx context.Context, book Book) error {
	if !provider.HasSession() {
		return ErrLoginRequired
	}
	book.ID = strings.TrimSpace(book.ID)
	if book.ID == "" {
		return &ParseError{Message: "加入官网书架需要 book ID"}
	}
	inShelf, err := provider.InBookshelf(ctx, book.ID)
	if err != nil {
		return err
	}
	if inShelf {
		return nil
	}
	payload := map[string]any{
		"identify_data": []map[string]any{{
			"asterisked":  false,
			"modify_time": time.Now().UnixMilli(),
			"book_type":   book.BookType,
			"book_id":     book.ID,
		}},
		"add_book_source": 0,
	}
	if _, err = provider.authenticatedJSONPost(ctx, bookshelfAddURL+"?"+bookshelfParameters().Encode(), payload); err != nil {
		return err
	}
	inShelf, err = provider.InBookshelf(ctx, book.ID)
	if err != nil {
		return err
	}
	if !inShelf {
		return &ParseError{Message: "官网没有确认书籍已加入书架"}
	}
	return nil
}

// RemoveFromBookshelf removes a book from the account's official bookshelf.
// Fanqie's current website contract accepts the same identify_data shape used
// by its add route. The status checks make repeated calls idempotent and avoid
// issuing a destructive request for a book that is already absent.
func (provider *WebProvider) RemoveFromBookshelf(ctx context.Context, book Book) error {
	if !provider.HasSession() {
		return ErrLoginRequired
	}
	book.ID = strings.TrimSpace(book.ID)
	if book.ID == "" {
		return &ParseError{Message: "移出官网书架需要 book ID"}
	}
	inShelf, err := provider.InBookshelf(ctx, book.ID)
	if err != nil {
		return err
	}
	if !inShelf {
		return nil
	}
	payload := map[string]any{
		"identify_data": []map[string]any{{
			"asterisked":  false,
			"modify_time": time.Now().UnixMilli(),
			"book_type":   book.BookType,
			"book_id":     book.ID,
		}},
	}
	if _, err = provider.authenticatedJSONPost(ctx, bookshelfDeleteURL+"?"+bookshelfParameters().Encode(), payload); err != nil {
		return err
	}
	inShelf, err = provider.InBookshelf(ctx, book.ID)
	if err != nil {
		return err
	}
	if inShelf {
		return &ParseError{Message: "官网没有确认书籍已移出书架"}
	}
	return nil
}

// UpdateProgress writes the successfully opened chapter to the account's
// official reading history using the same payload as the website reader.
func (provider *WebProvider) UpdateProgress(ctx context.Context, bookID, chapterID string, order int) error {
	if !provider.HasSession() {
		return ErrLoginRequired
	}
	bookID, chapterID = strings.TrimSpace(bookID), strings.TrimSpace(chapterID)
	if bookID == "" || chapterID == "" || order < 0 {
		return &ParseError{Message: "同步官网进度缺少书籍或章节信息"}
	}
	payload := map[string]any{
		"book_id":        bookID,
		"item_id":        chapterID,
		"read_progress":  order,
		"index":          order,
		"read_timestamp": strconv.FormatInt(time.Now().Unix(), 10),
		"genre_type":     0,
	}
	_, err := provider.authenticatedJSONPost(ctx, updateProgressURL, payload)
	return err
}

// GetReviewFeed returns the latest official public review index. Fanqie's PC
// website does not expose a per-book list API, so this endpoint is presented as
// a latest-review feed instead of pretending to be exhaustive.
func (provider *WebProvider) GetReviewFeed(ctx context.Context) ([]ReviewSummary, error) {
	body, err := provider.request(ctx, commentSitemapURL)
	if err != nil {
		return nil, err
	}
	const marker = "window._SSR_DATA = "
	start := bytes.Index(body, []byte(marker))
	if start < 0 {
		return nil, &ParseError{Message: "官网书评索引缺少 SSR 数据"}
	}
	start += len(marker)
	end := bytes.Index(body[start:], []byte("</script>"))
	if end < 0 {
		return nil, &ParseError{Message: "官网书评索引数据不完整"}
	}
	decoder := json.NewDecoder(bytes.NewReader(body[start : start+end]))
	decoder.UseNumber()
	var state map[string]any
	if err := decoder.Decode(&state); err != nil {
		return nil, &ParseError{Message: "无法解析官网书评索引"}
	}
	data := object(state["data"])
	loaders := object(data["loadersData"])
	for _, loaderValue := range loaders {
		loader := object(loaderValue)
		page := object(loader["data"])
		rows, _ := page["links"].([]any)
		if len(rows) == 0 {
			continue
		}
		result := make([]ReviewSummary, 0, len(rows))
		for _, rowValue := range rows {
			row := object(rowValue)
			matches := commentURLPattern.FindStringSubmatch(asString(row["url"]))
			if len(matches) != 3 {
				continue
			}
			title := decrypt(asString(row["title"]))
			text, bookTitle := title, ""
			if split := strings.LastIndex(title, "_"); split > 0 {
				text, bookTitle = title[:split], title[split+1:]
			}
			result = append(result, ReviewSummary{BookID: matches[1], CommentID: matches[2], BookTitle: bookTitle, Text: text})
		}
		return result, nil
	}
	return nil, &ParseError{Message: "官网书评索引没有返回可用书评"}
}

// GetComment loads one complete official public book-review page.
func (provider *WebProvider) GetComment(ctx context.Context, bookID, commentID string) (CommentDetail, error) {
	bookID, commentID = strings.TrimSpace(bookID), strings.TrimSpace(commentID)
	if bookID == "" || commentID == "" {
		return CommentDetail{}, &ParseError{Message: "查看书评需要 book ID 和 comment ID"}
	}
	parameters := url.Values{"bookId": {bookID}, "commentId": {commentID}, "userId": {"0"}}
	payload, err := provider.json(ctx, commentDetailURL+"?"+parameters.Encode())
	if err != nil {
		return CommentDetail{}, err
	}
	data := object(payload["data"])
	base := object(data["base_resp"])
	if data == nil || (base != nil && asInt(base["code"], 0) != 0) {
		return CommentDetail{}, ErrNotFound
	}
	novel := object(data["novel"])
	book, ok := bookFromRow(novel)
	if !ok {
		return CommentDetail{}, &ParseError{Message: "书评接口缺少书籍信息"}
	}
	book.ReadCount = asInt(first(novel, "read_count", "readCount"), 0)
	book.BookType = asInt(first(novel, "book_type", "type"), 0)
	book.Status = firstNonEmptyString(decrypt(asString(novel["finish_status"])), book.Status)
	detail := CommentDetail{Book: book}
	detail.Reviews = reviewsFromRows(data["comments"])
	detail.Replies = reviewsFromRows(data["replies"])
	return detail, nil
}

func reviewsFromRows(value any) []Review {
	rows, _ := value.([]any)
	reviews := make([]Review, 0, len(rows))
	for _, rowValue := range rows {
		row := object(rowValue)
		info, user := object(row["info"]), object(row["user"])
		if info == nil {
			continue
		}
		score, scoreKnown := firstPresent(info, "score")
		review := Review{
			ID:           asString(info["comment_id"]),
			UserName:     decrypt(asString(first(user, "nick_name", "name"))),
			Text:         decrypt(asString(info["text"])),
			Score:        asFloat(score),
			ScoreKnown:   scoreKnown && asFloat(score) > 0,
			Likes:        asInt(info["digg_count"], 0),
			ReplyCount:   asInt(info["reply_count"], 0),
			ReadDuration: time.Duration(asInt(info["read_duration"], 0)) * time.Second,
			CreatedAt:    unixTime(info["create_time"]),
		}
		if review.ID != "" && review.Text != "" {
			reviews = append(reviews, review)
		}
	}
	return reviews
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// GetCategories returns the current public category catalog.
func (provider *WebProvider) GetCategories(ctx context.Context) ([]Category, error) {
	parameters := url.Values{"config_key": {"serial_rank_category_list_common"}}
	payload, err := provider.json(ctx, categoryConfigURL+"?"+parameters.Encode())
	if err != nil {
		return nil, err
	}
	data := object(payload["data"])
	if data == nil {
		return nil, &ParseError{Message: "分类接口缺少 data"}
	}
	rows, _ := data["list"].([]any)
	categories := make([]Category, 0, len(rows))
	for _, value := range rows {
		row := object(value)
		if row == nil {
			continue
		}
		id := asString(row["id"])
		name := decrypt(asString(row["name"]))
		groups, _ := row["group"].([]any)
		for _, group := range groups {
			gender := strings.ToLower(asString(group))
			if id != "" && name != "" && (gender == "male" || gender == "female") {
				categories = append(categories, Category{ID: id, Name: name, Gender: gender})
			}
		}
	}
	if len(categories) == 0 {
		return nil, &ParseError{Message: "分类接口没有返回可用分类"}
	}
	return categories, nil
}

// GetCategoryRank returns one page of a public category ranking. The ranking
// version changes daily, so it is read from the server-rendered rank page for
// every request instead of being compiled into the client.
func (provider *WebProvider) GetCategoryRank(ctx context.Context, categoryID, gender string, offset int) (SearchPage, error) {
	categoryID = strings.TrimSpace(categoryID)
	gender = strings.ToLower(strings.TrimSpace(gender))
	if categoryID == "" || (gender != "male" && gender != "female") {
		return SearchPage{}, &ParseError{Message: "分类榜参数无效"}
	}
	pageBody, err := provider.request(ctx, rankPageURL)
	if err != nil {
		return SearchPage{}, err
	}
	match := rankVersionPattern.FindSubmatch(pageBody)
	if len(match) != 2 {
		return SearchPage{}, &ParseError{Message: "排行榜页面缺少动态版本"}
	}
	const pageSize = 20
	genderValue := "1"
	if gender == "female" {
		genderValue = "0"
	}
	parameters := url.Values{
		"app_id":         {"2503"},
		"rank_list_type": {"3"},
		"offset":         {strconv.Itoa(max(0, offset))},
		"limit":          {strconv.Itoa(pageSize)},
		"category_id":    {categoryID},
		"rank_version":   {string(match[1])},
		"gender":         {genderValue},
		"rankMold":       {"1"},
	}
	payload, err := provider.json(ctx, categoryRankURL+"?"+parameters.Encode())
	if err != nil {
		return SearchPage{}, err
	}
	data := object(payload["data"])
	if data == nil {
		return SearchPage{}, &ParseError{Message: "分类榜接口缺少 data"}
	}
	total := asInt(data["total_num"], 0)
	return booksPage(data["book_list"], max(0, offset), total > offset, total), nil
}

// GetTopAuthors returns public highlighted writers.
func (provider *WebProvider) GetTopAuthors(ctx context.Context) ([]Author, error) {
	payload, err := provider.json(ctx, topAuthorsURL)
	if err != nil {
		return nil, err
	}
	rows, _ := payload["author_list"].([]any)
	authors := make([]Author, 0, len(rows))
	for _, value := range rows {
		row := object(value)
		if row == nil {
			continue
		}
		author := Author{
			ID:           asString(first(row, "author_id", "authorId", "id")),
			Name:         decrypt(asString(first(row, "author", "name", "username"))),
			Level:        decrypt(asString(first(row, "author_level", "level"))),
			Introduction: decrypt(asString(first(row, "introduction", "description"))),
			AvatarURL:    asString(first(row, "avator_url", "avatar_url", "avatar")),
		}
		if author.ID != "" && author.Name != "" {
			authors = append(authors, author)
		}
	}
	if len(authors) == 0 {
		return nil, &ParseError{Message: "热门作者接口没有返回可用作者"}
	}
	return authors, nil
}

// GetAuthor returns one public writer profile and all works exposed by the
// writer homepage.
func (provider *WebProvider) GetAuthor(ctx context.Context, authorID string) (AuthorProfile, []Book, error) {
	authorID = strings.TrimSpace(authorID)
	if authorID == "" {
		return AuthorProfile{}, nil, &ParseError{Message: "作者 ID 为空"}
	}
	parameters := url.Values{"id": {authorID}}
	profilePayload, err := provider.json(ctx, writerInfoURL+"?"+parameters.Encode())
	if err != nil {
		return AuthorProfile{}, nil, err
	}
	data := object(profilePayload["data"])
	if data == nil {
		return AuthorProfile{}, nil, &ParseError{Message: "作者接口缺少 data"}
	}
	profile := AuthorProfile{
		Author: Author{
			ID:           authorID,
			Name:         decrypt(asString(first(data, "name", "username"))),
			Introduction: decrypt(asString(first(data, "description", "introduction"))),
			AvatarURL:    asString(first(data, "avatar", "avatar_url")),
		},
		Followers:    decrypt(asString(data["fans_count"])) + decrypt(asString(data["fans_count_unit"])),
		WordCount:    decrypt(asString(data["word_count"])) + decrypt(asString(data["word_count_unit"])),
		CreationDays: decrypt(asString(data["creation_days"])),
	}
	booksPayload, err := provider.json(ctx, writerBooksURL+"?"+parameters.Encode())
	if err != nil {
		return AuthorProfile{}, nil, err
	}
	bookData := object(booksPayload["data"])
	if bookData == nil {
		return AuthorProfile{}, nil, &ParseError{Message: "作者作品接口缺少 data"}
	}
	page := booksPage(bookData["book_list"], 0, false, asInt(bookData["total_count"], 0))
	return profile, page.Books, nil
}

func firstPresent(row map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := row[key]; ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func rowsFromData(value any) ([]any, bool) {
	if rows, ok := value.([]any); ok {
		return rows, true
	}
	data := object(value)
	if data == nil {
		return nil, false
	}
	for _, key := range []string{"list", "items", "progress_list", "read_item_list"} {
		if rows, ok := data[key].([]any); ok {
			return rows, true
		}
	}
	return nil, false
}

func unixTime(value any) time.Time {
	seconds := asFloat(value)
	if seconds <= 0 {
		return time.Time{}
	}
	if seconds >= 1e12 {
		seconds /= 1000
	}
	whole := int64(seconds)
	nanoseconds := int64((seconds - float64(whole)) * float64(time.Second))
	return time.Unix(whole, nanoseconds).UTC()
}

func booksPage(value any, offset int, mayHaveMore bool, total int) SearchPage {
	page := SearchPage{NextOffset: offset, HasMore: mayHaveMore}
	rows, _ := value.([]any)
	for _, value := range rows {
		row := object(value)
		if row == nil {
			continue
		}
		book, ok := bookFromRow(row)
		if !ok {
			continue
		}
		if publisher := asString(row["publisher"]); publisher != "" {
			book.Status = strings.Join(nonEmptyStrings(asString(row["progress"]), publisher), " · ")
		}
		resultCategory := asString(first(row, "book_type", "category"))
		if resultCategory != "" {
			book.Category = resultCategory
		}
		page.Books = append(page.Books, book)
	}
	page.NextOffset = offset + len(rows)
	if total > 0 {
		page.HasMore = page.NextOffset < total
	} else if len(page.Books) == 0 {
		page.HasMore = false
	}
	return page
}

func bookFromRow(row map[string]any) (Book, bool) {
	bookID := asString(first(row, "book_id", "bookId"))
	title := asString(first(row, "book_name", "bookName", "title"))
	if bookID == "" || title == "" {
		return Book{}, false
	}
	return Book{
		ID:             bookID,
		Title:          decrypt(title),
		Author:         decrypt(asString(first(row, "author", "author_name"))),
		Abstract:       decrypt(asString(first(row, "abstract", "book_abstract"))),
		CoverURL:       asString(first(row, "thumb_url", "thumbUri", "thumb_uri", "cover", "cover_url")),
		Category:       decrypt(asString(first(row, "complete_category", "category", "genre"))),
		WordCount:      asInt(first(row, "word_number", "word_count"), 0),
		ChapterCount:   asInt(first(row, "chapter_number", "chapter_count"), 0),
		Score:          asFloat(row["score"]),
		ReadCount:      asInt(first(row, "read_count", "readCount"), 0),
		BookshelfCount: asInt(first(row, "add_bookshelf_count", "bookshelf_count"), 0),
		BookType:       asInt(first(row, "book_type", "type"), 0),
		Status:         creationStatus(first(row, "creation_status", "creationStatus")),
	}, true
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
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
	book := Book{
		ID:           asString(first(page, "bookId", "book_id")),
		Title:        decrypt(asString(first(page, "bookName", "book_name"))),
		Author:       decrypt(asString(first(page, "authorName", "author", "author_name"))),
		Abstract:     decrypt(asString(first(page, "abstract", "description"))),
		CoverURL:     asString(first(page, "thumbUrl", "thumbUri", "thumb_url")),
		Category:     category(page),
		WordCount:    asInt(first(page, "wordNumber", "word_count"), 0),
		ChapterCount: asInt(first(page, "chapterTotal", "chapter_count"), 0),
		ReadCount:    asInt(first(page, "readCount", "read_count"), 0),
		BookType:     asInt(first(page, "type", "book_type"), 0),
		Status:       creationStatus(first(page, "creationStatus", "creation_status")),
	}
	// The public detail page deliberately omits the aggregate score. The
	// official search response contains it, so enrich the exact matching book.
	if search, searchErr := provider.Search(ctx, book.Title, 0); searchErr == nil {
		for _, candidate := range search.Books {
			if candidate.ID != book.ID {
				continue
			}
			book.Score = candidate.Score
			book.BookshelfCount = candidate.BookshelfCount
			if book.ReadCount == 0 {
				book.ReadCount = candidate.ReadCount
			}
			break
		}
	}
	return book, nil
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
			itemID := asString(first(row, "itemId", "item_id"))
			if itemID == "" {
				continue
			}
			fallbackOrder := len(chapters) + 1
			rowVolume := asString(first(row, "volume_name", "volumeName"))
			if rowVolume == "" {
				rowVolume = volumeName
			}
			chapters = append(chapters, Chapter{
				ID:      itemID,
				Title:   decrypt(asString(first(row, "title", "chapter_title"))),
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
