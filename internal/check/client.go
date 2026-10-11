package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func ValidateGateway(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("请输入有效 Magpie HTTP(S) 网关地址")
	}
	return nil
}

var client = &http.Client{Timeout: 180 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("不允许网关重定向") }}

type Model struct {
	ID    string `json:"id"`
	Group string `json:"group"`
}

func Models(ctx context.Context, c Config) ([]Model, error) {
	if err := ValidateGateway(c.GatewayURL); err != nil {
		return nil, err
	}
	base := strings.TrimRight(c.GatewayURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.GatewayKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("无法读取 Magpie 模型列表")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("模型列表 HTTP %d", resp.StatusCode)
	}
	var data struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(&data); err != nil {
		return nil, errors.New("模型列表无效")
	}
	out := []Model{}
	for _, m := range data.Data {
		group := m.OwnedBy
		if i := strings.Index(m.ID, "/"); i > 0 {
			group = m.ID[:i]
		}
		out = append(out, Model{ID: m.ID, Group: group})
	}
	return out, nil
}

func Call(ctx context.Context, c Config, model, prompt string) (string, int, error) {
	payload := map[string]any{"model": model, "stream": false, "max_tokens": 8192, "messages": []any{map[string]string{"role": "user", "content": prompt}}}
	b, _ := json.Marshal(payload)
	base := strings.TrimRight(c.GatewayURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.GatewayKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return "", 0, err
	}
	if len(raw) > 4*1024*1024 {
		return "", 0, errors.New("回复超过大小限制")
	}
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("Magpie HTTP %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Finish string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return "", 0, errors.New("无法解析完整回复")
	}
	if len(out.Choices) != 1 {
		return "", 0, errors.New("缺少唯一有效回答")
	}
	ch := out.Choices[0]
	if ch.Finish != "stop" {
		return ch.Message.Content, out.Usage.Total, errors.New("回复未正常结束或被截断")
	}
	if strings.TrimSpace(ch.Message.Content) == "" {
		return "", out.Usage.Total, errors.New("空回答")
	}
	return ch.Message.Content, out.Usage.Total, nil
}

var finalNumber = regexp.MustCompile(`(?i)(?:\*\*)?(\d+(?:\.\d+)?)(?:\*\*)?\s*(?:inches|inch|英寸)`)

func Grade(id, answer string) string {
	if id != "milk" {
		return "manual"
	}
	if strings.Trim(strings.TrimSpace(answer), "*` ") == "46.24" {
		return "passed"
	}
	matches := finalNumber.FindAllStringSubmatch(answer, -1)
	if len(matches) == 0 {
		return "failed"
	}
	if matches[len(matches)-1][1] == "46.24" {
		return "passed"
	}
	return "failed"
}
