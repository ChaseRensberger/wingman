package tool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/core"
)

const (
	maxResponseSize = 5 * 1024 * 1024
	defaultTimeout  = 30 * time.Second
	maxTimeout      = 120 * time.Second
)

type WebFetchTool struct{}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{}
}

func (t *WebFetchTool) Name() string {
	return "webfetch"
}

func (t *WebFetchTool) Description() string {
	return "Fetch content from a URL. Supports text, markdown, and html output formats. HTML is automatically converted to markdown by default."
}

func (t *WebFetchTool) Definition() core.ToolDefinition {
	return core.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: core.ToolInputSchema{
			Type: "object",
			Properties: map[string]core.ToolProperty{
				"url": {
					Type:        "string",
					Description: "The URL to fetch content from",
				},
				"format": {
					Type:        "string",
					Description: "The format to return the content in (text, markdown, or html). Defaults to markdown.",
					Enum:        []string{"text", "markdown", "html"},
				},
				"timeout": {
					Type:        "integer",
					Description: "Optional timeout in seconds (max 120)",
				},
			},
			Required: []string{"url"},
		},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	url, ok := params["url"].(string)
	if !ok || url == "" {
		return "", fmt.Errorf("url is required")
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}

	format := "markdown"
	if f, ok := params["format"].(string); ok && f != "" {
		format = f
	}

	timeout := defaultTimeout
	if t, ok := params["timeout"].(float64); ok {
		timeout = time.Duration(t) * time.Second
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	acceptHeader := buildAcceptHeader(format)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 && resp.Header.Get("cf-mitigated") == "challenge" {
		req, _ = http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("User-Agent", "wingman")
		req.Header.Set("Accept", acceptHeader)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		resp, err = client.Do(req)
		if err != nil {
			return "", fmt.Errorf("retry request failed: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if len(body) > maxResponseSize {
		return "", fmt.Errorf("response too large (exceeds 5MB limit)")
	}

	content := string(body)
	contentType := resp.Header.Get("Content-Type")

	switch format {
	case "markdown":
		if strings.Contains(contentType, "text/html") {
			return convertHTMLToMarkdown(content), nil
		}
		return content, nil

	case "text":
		if strings.Contains(contentType, "text/html") {
			return extractTextFromHTML(content), nil
		}
		return content, nil

	case "html":
		return content, nil

	default:
		return content, nil
	}
}

func buildAcceptHeader(format string) string {
	switch format {
	case "markdown":
		return "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1"
	case "text":
		return "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1"
	case "html":
		return "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, text/markdown;q=0.7, */*;q=0.1"
	default:
		return "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	}
}

func extractTextFromHTML(html string) string {
	html = removeTagWithContent(html, "script")
	html = removeTagWithContent(html, "style")
	html = removeTagWithContent(html, "noscript")
	html = removeTagWithContent(html, "iframe")
	html = removeTagWithContent(html, "object")
	html = removeTagWithContent(html, "embed")

	tagRe := regexp.MustCompile(`<[^>]+>`)
	text := tagRe.ReplaceAllString(html, " ")

	whitespaceRe := regexp.MustCompile(`\s+`)
	text = whitespaceRe.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

func convertHTMLToMarkdown(html string) string {
	html = removeTagWithContent(html, "script")
	html = removeTagWithContent(html, "style")
	html = removeTagWithContent(html, "noscript")
	html = removeTagWithContent(html, "meta")
	html = removeTagWithContent(html, "link")

	selfClosingRe := regexp.MustCompile(`(?i)<(script|style|meta|link)[^>]*/>`)
	html = selfClosingRe.ReplaceAllString(html, "")

	h1Re := regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	html = h1Re.ReplaceAllString(html, "\n# $1\n")

	h2Re := regexp.MustCompile(`(?is)<h2[^>]*>(.*?)</h2>`)
	html = h2Re.ReplaceAllString(html, "\n## $1\n")

	h3Re := regexp.MustCompile(`(?is)<h3[^>]*>(.*?)</h3>`)
	html = h3Re.ReplaceAllString(html, "\n### $1\n")

	h4Re := regexp.MustCompile(`(?is)<h4[^>]*>(.*?)</h4>`)
	html = h4Re.ReplaceAllString(html, "\n#### $1\n")

	h5Re := regexp.MustCompile(`(?is)<h5[^>]*>(.*?)</h5>`)
	html = h5Re.ReplaceAllString(html, "\n##### $1\n")

	h6Re := regexp.MustCompile(`(?is)<h6[^>]*>(.*?)</h6>`)
	html = h6Re.ReplaceAllString(html, "\n###### $1\n")

	pRe := regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`)
	html = pRe.ReplaceAllString(html, "\n$1\n")

	brRe := regexp.MustCompile(`(?i)<br\s*/?>`)
	html = brRe.ReplaceAllString(html, "\n")

	hrRe := regexp.MustCompile(`(?i)<hr\s*/?>`)
	html = hrRe.ReplaceAllString(html, "\n---\n")

	strongRe := regexp.MustCompile(`(?is)<strong[^>]*>(.*?)</strong>`)
	html = strongRe.ReplaceAllString(html, "**$1**")

	bRe := regexp.MustCompile(`(?is)<b[^>]*>(.*?)</b>`)
	html = bRe.ReplaceAllString(html, "**$1**")

	emRe := regexp.MustCompile(`(?is)<em[^>]*>(.*?)</em>`)
	html = emRe.ReplaceAllString(html, "*$1*")

	iRe := regexp.MustCompile(`(?is)<i[^>]*>(.*?)</i>`)
	html = iRe.ReplaceAllString(html, "*$1*")

	codeRe := regexp.MustCompile(`(?is)<code[^>]*>(.*?)</code>`)
	html = codeRe.ReplaceAllString(html, "`$1`")

	preRe := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)
	html = preRe.ReplaceAllString(html, "\n```\n$1\n```\n")

	linkRe := regexp.MustCompile(`(?is)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	html = linkRe.ReplaceAllString(html, "[$2]($1)")

	imgRe := regexp.MustCompile(`(?i)<img[^>]*src="([^"]*)"[^>]*alt="([^"]*)"[^>]*/?>`)
	html = imgRe.ReplaceAllString(html, "![$2]($1)")

	imgNoAltRe := regexp.MustCompile(`(?i)<img[^>]*src="([^"]*)"[^>]*/?>`)
	html = imgNoAltRe.ReplaceAllString(html, "![]($1)")

	liRe := regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	html = liRe.ReplaceAllString(html, "- $1\n")

	ulRe := regexp.MustCompile(`(?i)</?ul[^>]*>`)
	html = ulRe.ReplaceAllString(html, "\n")

	olRe := regexp.MustCompile(`(?i)</?ol[^>]*>`)
	html = olRe.ReplaceAllString(html, "\n")

	blockquoteRe := regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`)
	html = blockquoteRe.ReplaceAllString(html, "> $1\n")

	tagRe := regexp.MustCompile(`<[^>]+>`)
	html = tagRe.ReplaceAllString(html, "")

	html = strings.ReplaceAll(html, "&nbsp;", " ")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")

	multipleNewlinesRe := regexp.MustCompile(`\n{3,}`)
	html = multipleNewlinesRe.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}

func removeTagWithContent(html, tag string) string {
	re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
	return re.ReplaceAllString(html, "")
}
