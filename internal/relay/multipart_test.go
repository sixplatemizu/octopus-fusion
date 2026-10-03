package relay

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

// buildMultipartBody 构造含 model 文本字段与图片文件 part 的表单。
func buildMultipartBody(t *testing.T, model, stream string, image []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if model != "" {
		if err := mw.WriteField("model", model); err != nil {
			t.Fatal(err)
		}
	}
	if stream != "" {
		if err := mw.WriteField("stream", stream); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("image", "test.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(image); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

// readMultipartFields 回读表单各文本字段与文件内容。
func readMultipartFields(t *testing.T, body []byte, contentType string) (map[string]string, []byte) {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	fields := map[string]string{}
	var file []byte
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		if part.FileName() != "" {
			file, err = io.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			b, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				t.Fatal(err)
			}
			fields[part.FormName()] = string(b)
		}
		_ = part.Close()
	}
	return fields, file
}

func parseMediaType(contentType string) (string, map[string]string, error) {
	return mime.ParseMediaType(contentType)
}

func newMultipartRequest(t *testing.T, body []byte, contentType string) *httpclient.Request {
	t.Helper()
	return &httpclient.Request{
		Method:  http.MethodPost,
		URL:     "http://127.0.0.1/v1/images/edits",
		Headers: http.Header{"Content-Type": []string{contentType}},
		Body:    body,
	}
}

func TestBuildRawPassthroughURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		format  llm.APIFormat
		want    string
	}{
		{"普通地址补v1", "https://api.openai.com", llm.APIFormatOpenAIChatCompletion, "https://api.openai.com/v1/chat/completions"},
		{"v1结尾不重复", "https://api.openai.com/v1", llm.APIFormatOpenAIChatCompletion, "https://api.openai.com/v1/chat/completions"},
		{"##视为完整地址", "https://ark.cn-beijing.volces.com/api/v3##", llm.APIFormatOpenAIChatCompletion, "https://ark.cn-beijing.volces.com/api/v3"},
		{"images路径", "https://api.openai.com", llm.APIFormatOpenAIImageEdit, "https://api.openai.com/v1/images/edits"},
		{"embeddings路径", "https://api.openai.com/", llm.APIFormatOpenAIEmbedding, "https://api.openai.com/v1/embeddings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRawPassthroughURL(tc.baseURL, tc.format)
			if got != tc.want {
				t.Fatalf("url = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRequestMetadataMultipart(t *testing.T) {
	image := bytes.Repeat([]byte{0x89, 0x50}, 512)
	body, ct := buildMultipartBody(t, "mock-group", "true", image)
	metadata, err := parseRequestMetadata(newMultipartRequest(t, body, ct))
	if err != nil {
		t.Fatalf("parse multipart: %v", err)
	}
	if metadata.Model != "mock-group" {
		t.Fatalf("model = %q, want mock-group", metadata.Model)
	}
	if !metadata.Streaming {
		t.Fatal("streaming = false, want true")
	}

	// 缺 stream 字段时保持非流式。
	body, ct = buildMultipartBody(t, "mock-group", "", image)
	metadata, err = parseRequestMetadata(newMultipartRequest(t, body, ct))
	if err != nil {
		t.Fatalf("parse multipart without stream: %v", err)
	}
	if metadata.Streaming {
		t.Fatal("streaming = true, want false")
	}
}

func TestRewriteMultipartRequestModel(t *testing.T) {
	image := bytes.Repeat([]byte{0xAB, 0xCD}, 1024)
	body, ct := buildMultipartBody(t, "mock-group", "false", image)
	raw := newMultipartRequest(t, body, ct)
	if err := rewriteMultipartRequestModel(raw, "gpt-image-1"); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	fields, file := readMultipartFields(t, raw.Body, raw.Headers.Get("Content-Type"))
	if fields["model"] != "gpt-image-1" {
		t.Fatalf("model = %q, want gpt-image-1", fields["model"])
	}
	if fields["stream"] != "false" {
		t.Fatalf("stream = %q, want false (untouched)", fields["stream"])
	}
	if !bytes.Equal(file, image) {
		t.Fatal("image file content changed")
	}

	// 缺 model 字段时追加。
	body, ct = buildMultipartBody(t, "", "false", image)
	raw = newMultipartRequest(t, body, ct)
	if err := rewriteMultipartRequestModel(raw, "gpt-image-1"); err != nil {
		t.Fatalf("rewrite missing model: %v", err)
	}
	fields, _ = readMultipartFields(t, raw.Body, raw.Headers.Get("Content-Type"))
	if fields["model"] != "gpt-image-1" {
		t.Fatalf("model = %q, want appended gpt-image-1", fields["model"])
	}
}

func TestApplyChannelConfigSkipsMultipart(t *testing.T) {
	image := []byte{0x01, 0x02, 0x03}
	body, ct := buildMultipartBody(t, "mock-group", "false", image)
	raw := newMultipartRequest(t, body, ct)
	channel := model.Channel{ChannelConfig: model.ChannelConfig{ParamOverride: `{"temperature":0.1}`}}
	if err := applyChannelConfig(channel, raw); err != nil {
		t.Fatalf("applyChannelConfig: %v", err)
	}
	// multipart body 不得被 sjson 改动：仍能按原 boundary 解析且字段完整。
	fields, _ := readMultipartFields(t, raw.Body, ct)
	if fields["model"] != "mock-group" {
		t.Fatalf("model = %q, want untouched mock-group", fields["model"])
	}
	if !strings.Contains(raw.Headers.Get("Content-Type"), "multipart/form-data") {
		t.Fatalf("content-type = %q, want multipart", raw.Headers.Get("Content-Type"))
	}
}
