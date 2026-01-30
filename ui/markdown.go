package ui

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/muesli/reflow/wordwrap"
)

var (
	codeBlockRegex  = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	inlineCodeRegex = regexp.MustCompile("`([^`]+)`")
	boldRegex       = regexp.MustCompile("\\*\\*([^*]+)\\*\\*")
	italicRegex     = regexp.MustCompile("\\*([^*]+)\\*")
	headerRegex     = regexp.MustCompile("(?m)^(#{1,6})\\s+(.+)$")
	bulletRegex     = regexp.MustCompile("(?m)^[-*]\\s+(.+)$")
)

func RenderMarkdown(content string, s Styles, width int) string {
	result := content

	result = codeBlockRegex.ReplaceAllStringFunc(result, func(match string) string {
		parts := codeBlockRegex.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		lang := parts[1]
		code := strings.TrimSpace(parts[2])
		highlighted := highlightCode(code, lang)
		return "\n" + s.Code.Render(highlighted) + "\n"
	})

	result = inlineCodeRegex.ReplaceAllStringFunc(result, func(match string) string {
		parts := inlineCodeRegex.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		return s.Code.Render(parts[1])
	})

	result = boldRegex.ReplaceAllString(result, "\033[1m$1\033[0m")
	result = italicRegex.ReplaceAllString(result, "\033[3m$1\033[0m")

	result = headerRegex.ReplaceAllStringFunc(result, func(match string) string {
		parts := headerRegex.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		return s.Header.Render(parts[2])
	})

	result = bulletRegex.ReplaceAllString(result, "  • $1")

	if width > 0 {
		result = wordwrap.String(result, width-4)
	}

	return result
}

func highlightCode(code, lang string) string {
	if lang == "" {
		lang = "text"
	}

	if lang == "command" || lang == "bash" || lang == "sh" {
		lang = "bash"
	}

	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return code
	}

	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return code
	}

	return strings.TrimRight(buf.String(), "\n")
}
