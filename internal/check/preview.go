package check

import (
	"io"
	"net/http"
	"regexp"
	"strings"
)

var htmlFence = regexp.MustCompile("(?is)```(?:html|htm)?[ \\t]*\\r?\\n(.*?)\\r?\\n```")
var htmlStart = regexp.MustCompile(`(?i)<!doctype\s+html\b|<html\b|<head\b|<body\b|<(?:div|main|section|style|svg|canvas)\b`)

func extractHTML(answer string) string {
	for _, match := range htmlFence.FindAllStringSubmatch(answer, -1) {
		if htmlStart.MatchString(match[1]) {
			return match[1]
		}
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
	if source == "" {
		http.Error(w, "回答中未找到 HTML，请查看原始回答", http.StatusUnprocessableEntity)
		return
	}
	// Inline animation code is allowed only in an opaque-origin sandbox, never
	// in the console. This policy also survives removal of generated meta tags.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'; sandbox allow-scripts")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), fullscreen=()")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, source)
}
