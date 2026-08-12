"""Parsing helpers for Fanqie's server-rendered pages."""

from __future__ import annotations

import html as html_lib
import json
import re
from typing import Any

from .errors import ParseError

_INITIAL_STATE = re.compile(r"window\.__INITIAL_STATE__\s*=\s*")
_TAG = re.compile(r"<[^>]+>")
_PARAGRAPH_START = re.compile(r"<(?:p|div|li|h[1-6])\b[^>]*>", re.IGNORECASE)
_PARAGRAPH_END = re.compile(r"</(?:p|div|li|h[1-6])\s*>", re.IGNORECASE)
_BREAK = re.compile(r"<br\s*/?>", re.IGNORECASE)


def extract_initial_state(page_html: str) -> dict[str, Any]:
    """Extract the JSON assigned to ``window.__INITIAL_STATE__``.

    ``JSONDecoder.raw_decode`` correctly handles braces inside chapter text,
    unlike brace-counting implementations commonly found in older clients.
    """

    match = _INITIAL_STATE.search(page_html)
    if not match:
        raise ParseError("页面中没有 window.__INITIAL_STATE__，网页结构可能已经变化")
    try:
        value, _ = json.JSONDecoder().raw_decode(page_html[match.end() :])
    except json.JSONDecodeError as exc:
        raise ParseError("无法解析页面中的 __INITIAL_STATE__") from exc
    if not isinstance(value, dict):
        raise ParseError("页面中的 __INITIAL_STATE__ 不是对象")
    return value


def html_to_text(content: str) -> str:
    """Convert the small HTML subset used for chapter bodies to plain text."""

    text = _BREAK.sub("\n", content)
    text = _PARAGRAPH_START.sub("", text)
    text = _PARAGRAPH_END.sub("\n\n", text)
    text = _TAG.sub("", text)
    text = html_lib.unescape(text).replace("\r", "")
    text = "\n".join(line.rstrip() for line in text.splitlines())
    return re.sub(r"\n{3,}", "\n\n", text).strip()


def as_int(value: object, default: int = 0) -> int:
    try:
        return int(value or default)
    except (TypeError, ValueError):
        return default


def as_float(value: object, default: float = 0.0) -> float:
    try:
        return float(value or default)
    except (TypeError, ValueError):
        return default


def as_bool(value: object, default: bool = False) -> bool:
    """Coerce the boolean-like values used by upstream JSON responses."""

    if value is None:
        return default
    if isinstance(value, str):
        normalized = value.strip().lower()
        if normalized in {"", "0", "false", "no", "null", "none"}:
            return False
        if normalized in {"1", "true", "yes"}:
            return True
    if isinstance(value, (bool, int, float)):
        return bool(value)
    return default


def first(mapping: dict[str, Any], *keys: str, default: Any = "") -> Any:
    """Return the first present, non-None value among alternate field names."""

    for key in keys:
        if key in mapping and mapping[key] is not None:
            return mapping[key]
    return default
