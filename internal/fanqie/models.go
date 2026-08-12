// Package fanqie implements access to Fanqie's public web pages.
package fanqie

// SearchPage is a page of book search results.
type SearchPage struct {
	Books      []Book
	NextOffset int
	HasMore    bool
}

// Book contains provider-independent book metadata.
type Book struct {
	ID           string
	Title        string
	Author       string
	Abstract     string
	CoverURL     string
	Category     string
	WordCount    int
	ChapterCount int
	Score        float64
	Status       string
}

// Chapter describes an entry in a book directory.
type Chapter struct {
	ID      string
	Title   string
	Order   int
	Volume  string
	Locked  bool
	NeedPay bool
}

// ChapterContent contains a readable public chapter.
type ChapterContent struct {
	BookID     string
	ChapterID  string
	Title      string
	Content    string
	Order      int
	PreviousID string
	NextID     string
	Locked     bool
	NeedPay    bool
	BookName   string
}
