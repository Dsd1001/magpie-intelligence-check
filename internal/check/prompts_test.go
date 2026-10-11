package check

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserPromptIntegrity(t *testing.T) {
	expected := map[string]string{
		"milk":   "0a481d61d137f99c302647101a8a2f2f01c2254a4cbf7c5b5e726fae0fc0afae",
		"repeat": "c039aca05f8daad02cb61d97e7f687d1e2da46201b87800d5b72a43a029fb36c",
		"japan":  "786f0d61f337cbe9056863e1ffa482cbd55f513fc1f3f3722ff5ba1eb34fb704",
		"person": "6119f7358e9c7c816c3d23072d8cb0d16c148deb3a541fe5a87a678811186273",
		"html":   "5e6cdd9ac71df22bd0133a90b3320cb0bbd58f813ca1fd390b416bbfb27f64a3",
		"svg":    "366764bd310a89292780b953b12da3f90822314e3ce23e7a827bcf6d5a6cf5d5",
		"map":    "1b7cb2d3b9a93fc1e8a27d31847d965dc5e48c2737de97c3d4ea4f15d450d3a9",
	}
	if len(Prompts) != len(expected) {
		t.Fatal("题库数量改变")
	}
	for _, p := range Prompts {
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(p.Text))); got != expected[p.ID] {
			t.Errorf("%s: 原文发生变化，hash=%s", p.ID, got)
		}
	}
}

func TestPromptSentWithoutRewriting(t *testing.T) {
	for _, p := range Prompts {
		t.Run(p.ID, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []struct{ Role, Content string }
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != p.Text {
					t.Error("请求正文不是原始提示词")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
			}))
			defer up.Close()
			if _, _, err := Call(context.Background(), Config{GatewayURL: up.URL, GatewayKey: "test"}, "a/model", p.Text); err != nil {
				t.Fatal(err)
			}
		})
	}
}
