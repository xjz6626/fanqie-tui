// Package fanqie implements access to Fanqie's public web pages.
package fanqie

import "time"

// DiscoverKind identifies a public book-discovery feed.
type DiscoverKind string

const (
	DiscoverPopular     DiscoverKind = "popular"
	DiscoverRecommended DiscoverKind = "recommended"
	DiscoverMale        DiscoverKind = "male"
	DiscoverFemale      DiscoverKind = "female"
	DiscoverRecent      DiscoverKind = "recent"
	DiscoverPublished   DiscoverKind = "published"
)

// SearchPage is a page of book search results.
type SearchPage struct {
	Books      []Book
	NextOffset int
	HasMore    bool
}

// Book contains provider-independent book metadata.
type Book struct {
	ID             string
	Title          string
	Author         string
	Abstract       string
	CoverURL       string
	Category       string
	WordCount      int
	ChapterCount   int
	Score          float64
	ReadCount      int
	BookshelfCount int
	BookType       int
	Status         string
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

// Account describes the reader account attached to an imported web session.
type Account struct {
	ID        string
	Name      string
	AvatarURL string
	VIP       bool
	VIPKnown  bool
}

// CloudProgress is the last cloud reading position known for a book.
// The official website returns IDs rather than display titles for this API.
type CloudProgress struct {
	BookID        string
	ChapterID     string
	ChapterOrder  int
	ReadProgress  float64
	ProgressKnown bool
	UpdatedAt     time.Time
}

// ReadItem is a cloud reading position for one chapter in a book.
type ReadItem struct {
	ChapterID     string
	ChapterOrder  int
	ReadProgress  float64
	ProgressKnown bool
	UpdatedAt     time.Time
}

// Category is one public ranking category. Gender is "male" or "female";
// the same category ID may be exposed in both groups.
type Category struct {
	ID     string
	Name   string
	Gender string
}

// Author is a public writer profile returned by the website.
type Author struct {
	ID           string
	Name         string
	Level        string
	Introduction string
	AvatarURL    string
}

// AuthorProfile adds the public counters shown on an author's homepage.
type AuthorProfile struct {
	Author
	Followers    string
	WordCount    string
	CreationDays string
}

// ReviewSummary is one entry from the official website's public comment
// sitemap. The sitemap is a discovery feed; GetComment loads the complete
// review, follow-up review and replies.
type ReviewSummary struct {
	BookID    string
	CommentID string
	BookTitle string
	Text      string
}

// Review contains one public book review or reply.
type Review struct {
	ID           string
	UserName     string
	Text         string
	Score        float64
	ScoreKnown   bool
	Likes        int
	ReplyCount   int
	ReadDuration time.Duration
	CreatedAt    time.Time
}

// CommentDetail contains a public book review page and the aggregate book
// statistics returned alongside it by the official website.
type CommentDetail struct {
	Book    Book
	Reviews []Review
	Replies []Review
}
