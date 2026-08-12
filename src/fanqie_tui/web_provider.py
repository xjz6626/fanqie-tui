"""Direct provider for Fanqie's public, server-rendered web pages."""

from __future__ import annotations

import json
from typing import Any

import httpx

from .errors import LockedChapterError, NetworkError, NotFoundError, ParseError
from .font_decode import build_mapping, decrypt
from .models import Book, Chapter, ChapterContent, SearchPage
from .parsing import as_bool, as_float, as_int, extract_initial_state, first, html_to_text
from .provider import NovelProvider

SEARCH_URL = "https://novel.snssdk.com/api/novel/channel/homepage/search/search/v1/"
BOOK_URL = "https://fanqienovel.com/page/{book_id}"
DIRECTORY_URL = "https://fanqienovel.com/api/reader/directory/detail"
CHAPTER_URL = "https://fanqienovel.com/reader/{item_id}"


def _creation_status(value: object) -> str:
    normalized = str(value) if value is not None else ""
    return {"0": "连载", "1": "完结"}.get(normalized, normalized)


def _category(page: dict[str, Any]) -> str:
    direct = str(first(page, "completeCategory", "category"))
    if direct:
        return direct
    raw = page.get("categoryV2")
    if not isinstance(raw, str):
        return ""
    try:
        categories = json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        return ""
    if not isinstance(categories, list):
        return ""
    names = [str(item["Name"]) for item in categories if isinstance(item, dict) and item.get("Name")]
    return " / ".join(names[:3])


class WebProvider(NovelProvider):
    """Read public metadata and unlocked chapters without an API key."""

    def __init__(self, timeout: float = 20.0, client: httpx.Client | None = None):
        self._owns_client = client is None
        self.client = client or httpx.Client(
            timeout=timeout,
            follow_redirects=True,
            transport=httpx.HTTPTransport(retries=2),
            headers={
                "User-Agent": (
                    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
                    "(KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"
                ),
                "Accept-Language": "zh-CN,zh;q=0.9",
            },
        )

    def close(self) -> None:
        if self._owns_client:
            self.client.close()

    def __enter__(self) -> "WebProvider":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()

    def _response(self, url: str, *, params: dict[str, object] | None = None) -> httpx.Response:
        try:
            response = self.client.get(url, params=params)
            if response.status_code == 404:
                raise NotFoundError("番茄小说返回 404，书籍或章节可能已下架")
            response.raise_for_status()
            return response
        except (NotFoundError, LockedChapterError):
            raise
        except httpx.HTTPStatusError as exc:
            raise NetworkError(f"上游返回 HTTP {exc.response.status_code}") from exc
        except httpx.HTTPError as exc:
            raise NetworkError(f"无法连接番茄小说：{exc}") from exc

    def _json(self, url: str, *, params: dict[str, object]) -> dict[str, Any]:
        response = self._response(url, params=params)
        try:
            payload = response.json()
        except ValueError as exc:
            raise ParseError("上游没有返回 JSON") from exc
        if not isinstance(payload, dict):
            raise ParseError("上游 JSON 格式异常")
        code = payload.get("code", 0)
        if code not in (0, 200):
            raise ParseError(f"上游接口错误 {code}: {payload.get('message', '未知错误')}")
        return payload

    def _page(self, url: str) -> tuple[str, dict[str, Any]]:
        page_html = self._response(url).text
        return page_html, extract_initial_state(page_html)

    def _font_mapping(self, page_html: str, state: dict[str, Any]) -> dict[str, str]:
        return build_mapping(page_html, state, lambda url: self._response(url).content)

    def search(self, query: str, offset: int = 0) -> SearchPage:
        if not query.strip():
            return SearchPage([])
        payload = self._json(SEARCH_URL, params={"q": query, "aid": 1967, "offset": offset})
        data = payload.get("data") or {}
        if not isinstance(data, dict):
            raise ParseError("搜索结果缺少 data")
        rows = data.get("ret_data") or []
        books: list[Book] = []
        for row in rows:
            if not isinstance(row, dict):
                continue
            books.append(
                Book(
                    id=str(first(row, "book_id", "bookId")),
                    title=str(first(row, "title", "book_name", "bookName")),
                    author=str(first(row, "author", "author_name")),
                    abstract=str(first(row, "abstract", "book_abstract")),
                    cover_url=str(first(row, "thumb_url", "cover")),
                    category=str(first(row, "category")),
                    score=as_float(first(row, "score")),
                    status=_creation_status(first(row, "creation_status")),
                )
            )
        return SearchPage(
            books=books,
            next_offset=as_int(data.get("offset"), offset + len(books)),
            has_more=as_bool(data.get("has_more")),
        )

    def get_book(self, book_id: str) -> Book:
        page_html, state = self._page(BOOK_URL.format(book_id=book_id))
        page = state.get("page")
        if not isinstance(page, dict) or not first(page, "bookId", "book_id"):
            raise ParseError("书籍页面缺少书籍信息")
        mapping = self._font_mapping(page_html, state)
        title = decrypt(str(first(page, "bookName", "book_name")), mapping)
        return Book(
            id=str(first(page, "bookId", "book_id", default=book_id)),
            title=title,
            author=decrypt(str(first(page, "authorName", "author", "author_name")), mapping),
            abstract=decrypt(str(first(page, "abstract", "description")), mapping),
            cover_url=str(first(page, "thumbUrl", "thumbUri", "thumb_url")),
            category=_category(page),
            word_count=as_int(first(page, "wordNumber", "word_count")),
            chapter_count=as_int(first(page, "chapterTotal", "chapter_count")),
            status=_creation_status(first(page, "creationStatus", "creation_status")),
        )

    def get_directory(self, book_id: str) -> list[Chapter]:
        payload = self._json(DIRECTORY_URL, params={"bookId": book_id})
        data = payload.get("data") or {}
        if not isinstance(data, dict):
            raise ParseError("目录结果缺少 data")
        volumes = data.get("chapterListWithVolume") or data.get("chapter_list_with_volume") or []
        chapters: list[Chapter] = []
        fallback_order = 0
        for volume in volumes:
            volume_name = ""
            rows: object = volume
            if isinstance(volume, dict):
                volume_name = str(first(volume, "volumeName", "volume_name", "title"))
                rows = first(volume, "chapterList", "chapter_list", "chapters", default=[])
            if not isinstance(rows, list):
                continue
            for row in rows:
                if not isinstance(row, dict):
                    continue
                fallback_order += 1
                chapters.append(
                    Chapter(
                        id=str(first(row, "itemId", "item_id")),
                        title=str(first(row, "title", "chapter_title")),
                        order=as_int(first(row, "realChapterOrder", "order"), fallback_order),
                        volume=str(first(row, "volume_name", "volumeName", default=volume_name)),
                        locked=as_bool(first(row, "isChapterLock", "is_chapter_lock", default=False)),
                        need_pay=as_bool(first(row, "needPay", "need_pay", default=False)),
                    )
                )
        if not chapters:
            raise ParseError("没有解析到章节，书籍可能已下架或网页结构已经变化")
        return chapters

    def get_chapter(self, item_id: str) -> ChapterContent:
        page_html, state = self._page(CHAPTER_URL.format(item_id=item_id))
        reader = state.get("reader")
        data = reader.get("chapterData") if isinstance(reader, dict) else None
        if not isinstance(data, dict) or not data:
            raise ParseError("章节页面缺少 chapterData")
        locked = as_bool(first(data, "isChapterLock", "is_chapter_lock", default=False))
        need_pay = as_bool(first(data, "needPay", "need_pay", default=False))
        if locked or need_pay:
            raise LockedChapterError("该章节已锁定或需要付费，请使用官方客户端阅读")
        raw_content = str(data.get("content") or "")
        if not raw_content:
            raise ParseError("章节正文为空，可能需要登录、验证或已被下架")
        mapping = self._font_mapping(page_html, state)
        return ChapterContent(
            book_id=str(first(data, "bookId", "book_id")),
            chapter_id=str(first(data, "itemId", "item_id", default=item_id)),
            title=decrypt(str(first(data, "title", "chapterTitle")), mapping),
            content=decrypt(html_to_text(raw_content), mapping),
            order=as_int(first(data, "realChapterOrder", "order")),
            previous_id=str(first(data, "preItemId", "pre_item_id")),
            next_id=str(first(data, "nextItemId", "next_item_id")),
            locked=locked,
            need_pay=need_pay,
            metadata={"book_name": first(data, "bookName", "book_name")},
        )
