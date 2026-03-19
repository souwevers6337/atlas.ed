package editor

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

var (
	searchMatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#D4AF37")).
				Foreground(lipgloss.Color("#000000"))

	currentMatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#FFFFFF")).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)
)

func Highlight(content, filename string) (string, error) {
	lexer := lexers.Get(filename)
	if lexer == nil {
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("monokai")
	formatter := formatters.Get("terminal256")

	iterator, _ := lexer.Tokenise(nil, content)
	var buf bytes.Buffer
	formatter.Format(&buf, style, iterator)

	return buf.String(), nil
}

func HighlightSearch(highlighted, query string, targetIdx int) string {
	if query == "" {
		return highlighted
	}
	
	lowerQuery := strings.ToLower(query)
	var result strings.Builder
	cursor := 0
	matchCounter := 0
	
	// Pre-size builder to avoid re-allocations
	result.Grow(len(highlighted) + 100)

	for cursor < len(highlighted) {
		start := strings.Index(highlighted[cursor:], "\x1b[")
		if start == -1 {
			res, count := highlightPlainPart(highlighted[cursor:], lowerQuery, targetIdx, matchCounter)
			result.WriteString(res)
			matchCounter = count
			break
		}
		
		start += cursor
		if start > cursor {
			res, count := highlightPlainPart(highlighted[cursor:start], lowerQuery, targetIdx, matchCounter)
			result.WriteString(res)
			matchCounter = count
		}
		
		end := strings.IndexAny(highlighted[start:], "mABCDHJKfhnpsu")
		if end == -1 {
			result.WriteString(highlighted[start:])
			break
		}
		end += start + 1
		result.WriteString(highlighted[start:end])
		cursor = end
	}
	
	return result.String()
}

func highlightPlainPart(text, lowerQuery string, targetIdx, currentCount int) (string, int) {
	if lowerQuery == "" || text == "" {
		return text, currentCount
	}

	lowerText := strings.ToLower(text)
	var result strings.Builder
	cursor := 0
	count := currentCount
	
	for {
		idx := strings.Index(lowerText[cursor:], lowerQuery)
		if idx == -1 {
			result.WriteString(text[cursor:])
			break
		}

		idx += cursor
		result.WriteString(text[cursor:idx])

		matchText := text[idx : idx+len(lowerQuery)]
		style := searchMatchStyle
		if count == targetIdx {
			style = currentMatchStyle
		}
		result.WriteString(style.Render(matchText))

		count++
		cursor = idx + len(lowerQuery)
	}
	return result.String(), count
}
