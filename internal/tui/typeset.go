package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const paragraphIndent = "　　"

const (
	forbiddenLineStart = "，。！？；：、,.!?;:%％）》】〕〉」』”’）］｝…"
	forbiddenLineEnd   = "（《【〔〈「『“‘([{［｛"
)

type textUnit struct {
	text  string
	width int
}

// typesetChineseProse normalizes paragraphs and wraps them according to the
// most visible Chinese line-breaking rules. It keeps closing punctuation off
// the beginning of a line and opening punctuation off the end of a line.
func typesetChineseProse(text string, width int) string {
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\u00a0", " ")
	lines := strings.Split(text, "\n")
	paragraphs := make([]string, 0, len(lines))
	blank := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank {
				paragraphs = append(paragraphs, "")
				blank = true
			}
			continue
		}
		paragraphs = append(paragraphs, line)
		blank = false
	}
	for len(paragraphs) > 0 && paragraphs[len(paragraphs)-1] == "" {
		paragraphs = paragraphs[:len(paragraphs)-1]
	}

	formatted := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			formatted = append(formatted, "")
			continue
		}
		formatted = append(formatted, wrapChineseLine(paragraphIndent+paragraph, width))
	}
	return strings.Join(formatted, "\n")
}

func wrapChineseLine(text string, width int) string {
	if width < 1 || ansi.StringWidth(text) <= width {
		return text
	}
	units := fitTextUnits(textUnits(text), width)
	lines := make([]string, 0, len(units)/max(1, width/2)+1)
	line := make([]textUnit, 0, width)
	lineWidth := 0
	flush := func() {
		if len(line) == 0 {
			return
		}
		var builder strings.Builder
		for _, unit := range line {
			builder.WriteString(unit.text)
		}
		lines = append(lines, strings.TrimRight(builder.String(), " \t"))
		line = line[:0]
		lineWidth = 0
	}

	for _, unit := range units {
		if lineWidth+unit.width <= width || len(line) == 0 {
			line = append(line, unit)
			lineWidth += unit.width
			continue
		}

		carry := make([]textUnit, 0, 2)
		if width >= 4 && startsWithForbidden(unit.text, forbiddenLineStart) {
			for len(line) > 0 && strings.TrimSpace(line[len(line)-1].text) == "" {
				lineWidth -= line[len(line)-1].width
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				last := line[len(line)-1]
				line = line[:len(line)-1]
				lineWidth -= last.width
				carry = append(carry, last)
			}
		}
		for width >= 4 && len(line) > 1 && startsWithForbidden(line[len(line)-1].text, forbiddenLineEnd) {
			last := line[len(line)-1]
			line = line[:len(line)-1]
			lineWidth -= last.width
			carry = append([]textUnit{last}, carry...)
		}
		flush()
		line = append(line, carry...)
		for _, carried := range carry {
			lineWidth += carried.width
		}
		line = append(line, unit)
		lineWidth += unit.width
	}
	if len(line) > 0 {
		flush()
	}
	return strings.Join(lines, "\n")
}

func fitTextUnits(units []textUnit, width int) []textUnit {
	result := make([]textUnit, 0, len(units))
	for _, unit := range units {
		if unit.width <= width {
			result = append(result, unit)
			continue
		}
		text := unit.text
		for len(text) > 0 {
			cluster, clusterWidth := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
			if cluster == "" {
				break
			}
			result = append(result, textUnit{text: cluster, width: clusterWidth})
			text = text[len(cluster):]
		}
	}
	return result
}

func textUnits(text string) []textUnit {
	units := make([]textUnit, 0, utf8.RuneCountInString(text))
	for len(text) > 0 {
		cluster, width := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		if cluster == "" {
			break
		}
		text = text[len(cluster):]
		if isLatinWordCluster(cluster) {
			var word strings.Builder
			word.WriteString(cluster)
			wordWidth := width
			for len(text) > 0 {
				next, nextWidth := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
				if !isLatinWordCluster(next) {
					break
				}
				word.WriteString(next)
				wordWidth += nextWidth
				text = text[len(next):]
			}
			units = append(units, textUnit{text: word.String(), width: wordWidth})
			continue
		}
		units = append(units, textUnit{text: cluster, width: width})
	}
	return units
}

func isLatinWordCluster(cluster string) bool {
	runeValue, size := utf8.DecodeRuneInString(cluster)
	if runeValue == utf8.RuneError || size != len(cluster) || runeValue > unicode.MaxASCII {
		return false
	}
	return unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) || strings.ContainsRune("_'’", runeValue)
}

func startsWithForbidden(text, forbidden string) bool {
	runeValue, _ := utf8.DecodeRuneInString(text)
	return strings.ContainsRune(forbidden, runeValue)
}
