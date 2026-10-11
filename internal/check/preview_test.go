package check

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTMLPreviewIsolation(t *testing.T) {
	s := fixture(t)
	answer := "说明\n```html\n<!DOCTYPE html><html><body><canvas></canvas><script>window.x=1</script></body></html>\n```\n结束"
	s.state.Results = append(s.state.Results, Result{ID: "html", Prompt: "html", Answer: answer}, Result{ID: "svg", Prompt: "svg", Answer: "<svg/>"}, Result{ID: "empty", Prompt: "html", Answer: "no markup"})
	h := s.Handler("secret")
	for _, tc := range []struct {
		id, token string
		status    int
	}{{"html", "", 401}, {"html", "secret", 200}, {"missing", "secret", 404}, {"svg", "secret", 400}, {"empty", "secret", 422}} {
		r := httptest.NewRequest(http.MethodGet, "/api/results/"+tc.id+"/preview", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.id, w.Code)
		}
		if tc.status == 200 {
			csp := w.Header().Get("Content-Security-Policy")
			for _, directive := range []string{"sandbox allow-scripts", "connect-src 'none'", "frame-src 'none'", "base-uri 'none'", "form-action 'none'"} {
				if !strings.Contains(csp, directive) {
					t.Fatal(csp)
				}
			}
			if strings.Contains(csp, "allow-same-origin") || strings.Contains(w.Body.String(), "```") || !strings.Contains(w.Body.String(), "<script>") {
				t.Fatal("invalid sandbox or changed animation code")
			}
		}
	}
	if s.state.Results[0].Answer != answer {
		t.Fatal("preview modified original answer")
	}
}

func TestExtractHTML(t *testing.T) {
	for _, source := range []string{"```html\n<div>fragment</div>\n```", "说明 <!DOCTYPE html><html><body>page</body></html> tail", "```\n<canvas></canvas>\n```"} {
		got := extractHTML(source)
		if !strings.HasPrefix(got, "<") || strings.Contains(got, "```") || strings.Contains(got, "tail") {
			t.Fatal(got)
		}
	}
}
