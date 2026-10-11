package check

import (
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var htmlFence = regexp.MustCompile("(?is)```([a-z]*)[ \\t]*\\r?\\n(.*?)\\r?\\n```")
var htmlStart = regexp.MustCompile(`(?i)<!doctype\s+html\b|<(?:html|head|body|div|main|section|style|svg|canvas|p|h[1-6]|button|article|table|ul|ol|span|form|input|a|img)\b`)

func htmlCandidates(answer string) ([]string, bool) {
	var candidates []string
	var styles, scripts strings.Builder
	for _, match := range htmlFence.FindAllStringSubmatch(answer, -1) {
		switch strings.ToLower(match[1]) {
		case "", "html", "htm":
			if htmlStart.MatchString(match[2]) {
				candidates = append(candidates, match[2])
			}
		case "css":
			styles.WriteString("\n" + match[2])
		case "js", "javascript":
			scripts.WriteString("\n" + match[2])
		}
	}
	combined := len(candidates) > 0 && (styles.Len() > 0 || scripts.Len() > 0)
	if combined {
		candidates = append(candidates, "<style>"+styles.String()+"</style>"+candidates[0]+"<script>"+scripts.String()+"</script>")
	}
	return candidates, combined
}

func restrictHTMLNavigation(source string) (string, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode && (child.Data == "base" || child.Data == "meta") {
				remove := child.Data == "base"
				for _, attr := range child.Attr {
					if strings.EqualFold(attr.Key, "http-equiv") && strings.EqualFold(attr.Val, "refresh") {
						remove = true
					}
				}
				if remove {
					node.RemoveChild(child)
					child = next
					continue
				}
			}
			attrs := child.Attr[:0]
			for _, attr := range child.Attr {
				if attr.Key == "target" || attr.Key == "ping" || attr.Key == "action" || attr.Key == "formaction" {
					continue
				}
				if (child.Data == "a" || child.Data == "area") && attr.Key == "href" && !strings.HasPrefix(strings.TrimSpace(attr.Val), "#") {
					continue
				}
				attrs = append(attrs, attr)
			}
			child.Attr = attrs
			walk(child)
			child = next
		}
	}
	walk(doc)
	var out strings.Builder
	err = html.Render(&out, doc)
	return out.String(), err
}

func extractHTML(answer string) string {
	if candidates, _ := htmlCandidates(answer); len(candidates) > 0 {
		return candidates[0]
	}
	start := htmlStart.FindStringIndex(answer)
	if start == nil {
		return ""
	}
	answer = answer[start[0]:]
	if end := strings.Index(strings.ToLower(answer), "</html>"); end >= 0 {
		answer = answer[:end+7]
	}
	return answer
}

func (s *Service) htmlPreview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	var result Result
	for _, record := range s.state.Results {
		if record.ID == r.PathValue("id") {
			result = record
			break
		}
	}
	s.mu.Unlock()
	if result.ID == "" {
		http.NotFound(w, r)
		return
	}
	if result.Prompt != "html" {
		http.Error(w, "仅 HTML 题支持实时预览", http.StatusBadRequest)
		return
	}
	source := extractHTML(result.Answer)
	candidates, combined := htmlCandidates(result.Answer)
	if len(candidates) == 0 && source != "" {
		candidates = []string{source}
	}
	if value := r.URL.Query().Get("candidate"); value != "" {
		index, err := strconv.Atoi(value)
		if err != nil || index < 0 || index >= len(candidates) {
			http.Error(w, "无效预览代码块", http.StatusBadRequest)
			return
		}
		source = candidates[index]
	}
	if source == "" {
		http.Error(w, "回答中未找到 HTML，请查看原始回答", http.StatusUnprocessableEntity)
		return
	}
	source, err := restrictHTMLNavigation(source)
	if err != nil {
		http.Error(w, "无法解析 HTML 预览", 422)
		return
	}
	// Inline animation code is allowed only in an opaque-origin sandbox, never
	// in the console. This policy also survives removal of generated meta tags.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'; sandbox allow-scripts")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), fullscreen=()")
	w.Header().Set("X-Preview-Candidates", strconv.Itoa(len(candidates)))
	w.Header().Set("X-Preview-Combined", strconv.FormatBool(combined))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, source+`<script>parent.postMessage({type:"intelligence-preview-ready"},"*")</script>`)
}
