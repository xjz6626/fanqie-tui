from fanqie_tui.font_decode import fallback_mapping, find_font_url, decrypt


def test_fallback_mapping_decodes_known_private_character() -> None:
    mapping = fallback_mapping()
    private_character, plain_character = next(iter(mapping.items()))

    assert decrypt(f"甲{private_character}乙", mapping) == f"甲{plain_character}乙"


def test_find_font_url_prefers_initial_state_css() -> None:
    state = {"common": {"css": "src:url('https://static.example/font.woff2?v=3')"}}

    assert find_font_url("", state) == "https://static.example/font.woff2?v=3"


def test_unknown_private_character_is_left_untouched() -> None:
    assert decrypt("\uf8ff") == "\uf8ff"
