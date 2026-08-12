"""Provider-independent data models."""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(slots=True)
class SearchPage:
    books: list["Book"]
    next_offset: int = 0
    has_more: bool = False


@dataclass(slots=True)
class Book:
    id: str
    title: str
    author: str = ""
    abstract: str = ""
    cover_url: str = ""
    category: str = ""
    word_count: int = 0
    chapter_count: int = 0
    score: float = 0.0
    status: str = ""


@dataclass(slots=True)
class Chapter:
    id: str
    title: str
    order: int
    volume: str = ""
    locked: bool = False
    need_pay: bool = False


@dataclass(slots=True)
class ChapterContent:
    book_id: str
    chapter_id: str
    title: str
    content: str
    order: int = 0
    previous_id: str = ""
    next_id: str = ""
    locked: bool = False
    need_pay: bool = False
    metadata: dict[str, object] = field(default_factory=dict)
