from __future__ import annotations

import json

import httpx
import pytest

from fanqie_tui.errors import LockedChapterError, NotFoundError
from fanqie_tui.font_decode import fallback_mapping
from fanqie_tui.web_provider import WebProvider


def _page(state: dict[str, object]) -> str:
    return f"<html><script>window.__INITIAL_STATE__={json.dumps(state, ensure_ascii=False)};</script></html>"


@pytest.fixture
def provider() -> WebProvider:
    private_character, _ = next(iter(fallback_mapping().items()))

    def handler(request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path.endswith("/search/v1/"):
            return httpx.Response(
                200,
                json={
                    "code": 0,
                    "data": {
                        "ret_data": [
                            {
                                "book_id": "book-1",
                                "title": "示例书",
                                "author": "作者",
                                "category": "玄幻",
                                "creation_status": "0",
                                "score": "9.1",
                            }
                        ],
                        "offset": 10,
                        "has_more": "1",
                    },
                },
            )
        if path == "/page/book-1":
            return httpx.Response(
                200,
                text=_page(
                    {
                        "page": {
                            "bookId": "book-1",
                            "bookName": "示例书",
                            "author": "作者",
                            "abstract": "简介",
                            "thumbUri": "https://img.example/cover.jpg",
                            "categoryV2": '[{"Name":"玄幻"},{"Name":"穿越"}]',
                            "wordNumber": "12345",
                            "chapterTotal": "2",
                            "creationStatus": 1,
                        }
                    }
                ),
            )
        if path.endswith("/api/reader/directory/detail"):
            return httpx.Response(
                200,
                json={
                    "code": 0,
                    "data": {
                        "chapterListWithVolume": [
                            [
                                {
                                    "itemId": "chapter-1",
                                    "title": "第一章",
                                    "realChapterOrder": "1",
                                    "isChapterLock": "0",
                                    "needPay": "0",
                                    "volume_name": "第一卷",
                                }
                            ],
                            {
                                "volumeName": "第二卷",
                                "chapterList": [
                                    {
                                        "itemId": "chapter-2",
                                        "title": "第二章",
                                        "isChapterLock": "1",
                                    }
                                ],
                            },
                        ]
                    },
                },
            )
        if path == "/reader/chapter-1":
            return httpx.Response(
                200,
                text=_page(
                    {
                        "reader": {
                            "chapterData": {
                                "bookId": "book-1",
                                "itemId": "chapter-1",
                                "title": "第一章",
                                "content": f"<p>正文{private_character}</p><p>第二段</p>",
                                "realChapterOrder": "1",
                                "nextItemId": "chapter-2",
                                "isChapterLock": "0",
                                "needPay": 0,
                            }
                        }
                    }
                ),
            )
        if path == "/reader/locked":
            return httpx.Response(
                200,
                text=_page(
                    {
                        "reader": {
                            "chapterData": {
                                "itemId": "locked",
                                "content": "不可读",
                                "isChapterLock": True,
                            }
                        }
                    }
                ),
            )
        return httpx.Response(404)

    client = httpx.Client(transport=httpx.MockTransport(handler))
    instance = WebProvider(client=client)
    yield instance
    client.close()


def test_search(provider: WebProvider) -> None:
    page = provider.search("示例")

    assert page.has_more is True
    assert page.next_offset == 10
    assert page.books[0].status == "连载"


def test_get_book_parses_current_page_fields(provider: WebProvider) -> None:
    book = provider.get_book("book-1")

    assert book.title == "示例书"
    assert book.cover_url.endswith("cover.jpg")
    assert book.category == "玄幻 / 穿越"
    assert book.word_count == 12345
    assert book.status == "完结"


def test_get_directory_supports_both_volume_shapes(provider: WebProvider) -> None:
    chapters = provider.get_directory("book-1")

    assert [chapter.id for chapter in chapters] == ["chapter-1", "chapter-2"]
    assert chapters[0].locked is False
    assert chapters[1].locked is True
    assert chapters[1].volume == "第二卷"


def test_get_chapter_decodes_and_formats_content(provider: WebProvider) -> None:
    _, expected_character = next(iter(fallback_mapping().items()))

    chapter = provider.get_chapter("chapter-1")

    assert chapter.content == f"正文{expected_character}\n\n第二段"
    assert chapter.next_id == "chapter-2"


def test_locked_chapter_is_rejected(provider: WebProvider) -> None:
    with pytest.raises(LockedChapterError):
        provider.get_chapter("locked")


def test_404_is_domain_error(provider: WebProvider) -> None:
    with pytest.raises(NotFoundError):
        provider.get_book("missing")
