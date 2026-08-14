package fanqie

import "errors"

var (
	// ErrLocked is returned for chapters unavailable on the public website.
	ErrLocked = errors.New("该章节已锁定或需要付费，请使用官方客户端阅读")
	// ErrNotFound indicates a removed or unknown book/chapter.
	ErrNotFound = errors.New("番茄小说返回 404，书籍或章节可能已下架")
	// ErrLoginRequired indicates that an official website feature needs an
	// authenticated browser session.
	ErrLoginRequired = errors.New("该功能需要登录番茄小说")
)

// ParseError reports an incompatible upstream response.
type ParseError struct{ Message string }

func (e *ParseError) Error() string { return e.Message }
