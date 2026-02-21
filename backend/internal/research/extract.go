package research

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"rsc.io/pdf"
)

var errUnsupportedContentType = errors.New("unsupported content type")

func extractContent(contentType string, body []byte, maxRunes int) (title, text string, err error) {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, parseErr := mime.ParseMediaType(mediaType); parseErr == nil {
		mediaType = parsed
	}

	switch mediaType {
	case "text/html", "application/xhtml+xml":
		title, text, err = extractHTMLText(body)
	case "text/plain", "text/markdown", "text/csv":
		text = normalizeExtractedText(string(body))
	case "application/json":
		text, err = extractJSONText(body)
	case "application/pdf":
		text, err = extractPDFTextFromBody(body)
	default:
		if strings.HasPrefix(mediaType, "text/") {
			text = normalizeExtractedText(string(body))
			break
		}
		return "", "", errUnsupportedContentType
	}
	if err != nil {
		return "", "", err
	}
	title = trimToRunes(strings.TrimSpace(title), 240)
	text = trimToRunes(normalizeExtractedText(text), maxRunes)
	return title, text, nil
}

func extractJSONText(data []byte) (string, error) {
	if !json.Valid(data) {
		return normalizeExtractedText(string(data)), nil
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return "", err
	}
	return normalizeExtractedText(pretty.String()), nil
}

func extractPDFTextFromBody(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	var textBuilder strings.Builder
	runeCount := 0
	for pageNum := 1; pageNum <= reader.NumPage(); pageNum++ {
		page := reader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		content := page.Content()
		for _, item := range content.Text {
			chunk := strings.TrimSpace(item.S)
			if chunk == "" {
				continue
			}
			if textBuilder.Len() > 0 {
				textBuilder.WriteByte('\n')
				runeCount++
			}
			textBuilder.WriteString(chunk)
			runeCount += utf8.RuneCountInString(chunk)
			if runeCount >= 220_000 {
				return trimToRunes(textBuilder.String(), 220_000), nil
			}
		}
	}

	return normalizeExtractedText(textBuilder.String()), nil
}

func extractHTMLText(data []byte) (title, text string, err error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", "", err
	}

	title = strings.TrimSpace(findHTMLTitle(doc))
	var builder strings.Builder
	walkHTMLText(doc, false, &builder)
	return title, normalizeExtractedText(builder.String()), nil
}

func findHTMLTitle(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, "title") {
		return strings.TrimSpace(textFromNode(node))
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if value := findHTMLTitle(child); value != "" {
			return value
		}
	}
	return ""
}

func textFromNode(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(textFromNode(child))
		builder.WriteByte(' ')
	}
	return builder.String()
}

func walkHTMLText(node *html.Node, skip bool, out *strings.Builder) {
	if node == nil || out == nil {
		return
	}
	if node.Type == html.ElementNode {
		switch strings.ToLower(node.Data) {
		case "script", "style", "noscript", "svg", "iframe", "head":
			skip = true
		case "p", "div", "section", "article", "li", "h1", "h2", "h3", "h4", "h5", "h6", "br", "tr":
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
		}
	}
	if node.Type == html.TextNode && !skip {
		trimmed := strings.TrimSpace(node.Data)
		if trimmed != "" {
			out.WriteString(trimmed)
			out.WriteByte(' ')
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkHTMLText(child, skip, out)
	}
}

func normalizeExtractedText(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ToValidUTF8(normalized, "")

	lines := strings.Split(normalized, "\n")
	compact := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		compact = append(compact, strings.Join(strings.Fields(trimmed), " "))
	}
	return strings.TrimSpace(strings.Join(compact, "\n"))
}
