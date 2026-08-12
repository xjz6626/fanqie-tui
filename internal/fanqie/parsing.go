package fanqie

import (
	"encoding/json"
	"fmt"
	stdhtml "html"
	"regexp"
	"strconv"
	"strings"
)

const initialStateMarker = "window.__INITIAL_STATE__"

var (
	breakTag      = regexp.MustCompile(`(?i)<br\s*/?>`)
	paragraphOpen = regexp.MustCompile(`(?i)<(?:p|div|li|h[1-6])\b[^>]*>`)
	paragraphEnd  = regexp.MustCompile(`(?i)</(?:p|div|li|h[1-6])\s*>`)
	anyTag        = regexp.MustCompile(`<[^>]+>`)
	manyNewlines  = regexp.MustCompile(`\n{3,}`)
)

func extractInitialState(page string) (map[string]any, error) {
	marker := strings.Index(page, initialStateMarker)
	if marker < 0 {
		return nil, &ParseError{Message: "页面中没有 window.__INITIAL_STATE__，网页结构可能已经变化"}
	}
	rest := page[marker+len(initialStateMarker):]
	equals := strings.Index(rest, "=")
	if equals < 0 {
		return nil, &ParseError{Message: "无法解析页面中的 __INITIAL_STATE__"}
	}
	var state map[string]any
	if err := json.NewDecoder(strings.NewReader(rest[equals+1:])).Decode(&state); err != nil {
		return nil, &ParseError{Message: "无法解析页面中的 __INITIAL_STATE__"}
	}
	return state, nil
}

func htmlToText(content string) string {
	text := breakTag.ReplaceAllString(content, "\n")
	text = paragraphOpen.ReplaceAllString(text, "")
	text = paragraphEnd.ReplaceAllString(text, "\n\n")
	text = anyTag.ReplaceAllString(text, "")
	text = strings.ReplaceAll(stdhtml.UnescapeString(text), "\r", "")
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	return strings.TrimSpace(manyNewlines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func asInt(value any, fallback int) int {
	if value == nil {
		return fallback
	}
	number, err := strconv.Atoi(asString(value))
	if err != nil {
		return fallback
	}
	return number
}

func asFloat(value any) float64 {
	number, _ := strconv.ParseFloat(asString(value), 64)
	return number
}

func asBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case json.Number:
		return typed.String() != "0"
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		return normalized == "1" || normalized == "true" || normalized == "yes"
	default:
		return false
	}
}

func first(mapping map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := mapping[key]; exists && value != nil {
			return value
		}
	}
	return nil
}

func object(value any) map[string]any {
	mapping, _ := value.(map[string]any)
	return mapping
}

func creationStatus(value any) string {
	switch asString(value) {
	case "0":
		return "连载"
	case "1":
		return "完结"
	default:
		return asString(value)
	}
}

func category(page map[string]any) string {
	if direct := asString(first(page, "completeCategory", "category")); direct != "" {
		return direct
	}
	var categories []map[string]any
	if err := json.Unmarshal([]byte(asString(page["categoryV2"])), &categories); err != nil {
		return ""
	}
	names := make([]string, 0, 3)
	for _, item := range categories {
		if name := asString(item["Name"]); name != "" {
			names = append(names, name)
			if len(names) == 3 {
				break
			}
		}
	}
	return strings.Join(names, " / ")
}
