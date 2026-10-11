package check

import (
	"strings"
	"testing"
)

func TestStreamProgressAndCompletion(t *testing.T) {
	source := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"你\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"好\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"total_tokens\":8}}\n\ndata: [DONE]\n\n"
	var updates []string
	answer, tokens, err := readAnswerStream(strings.NewReader(source), func(s string) { updates = append(updates, s) })
	if err != nil || answer != "你好" || tokens != 8 || len(updates) != 2 || updates[0] != "你" {
		t.Fatalf("%q %d %v %v", answer, tokens, err, updates)
	}
	for _, incomplete := range []string{strings.Split(source, "data: {\"choices\":[],")[0], strings.ReplaceAll(source, "\"stop\"", "\"length\""), strings.ReplaceAll(source, "\"index\":0", "\"index\":1")} {
		answer, _, err := readAnswerStream(strings.NewReader(incomplete), func(string) {})
		if err == nil {
			t.Fatal("invalid stream accepted")
		}
		if !strings.Contains(incomplete, "\"index\":1") && answer != "你好" {
			t.Fatal("partial answer lost", answer)
		}
	}
}
