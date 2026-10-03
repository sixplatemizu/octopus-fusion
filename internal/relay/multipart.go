package relay

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"

	"github.com/looplj/axonhub/llm/httpclient"
)

// rewriteMultipartRequestModel 将 multipart 表单中的 model 文本字段替换为上游真实模型名。
// 其余 part（含图片文件）原样拷贝；替换后 boundary 会变化，Content-Type 随之更新。
// 每轮重试换渠道时真实模型名可能不同，故需在每轮发请求前调用。
func rewriteMultipartRequestModel(raw *httpclient.Request, newModel string) error {
	contentType := raw.Headers.Get("Content-Type")
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid multipart content type: %w", err)
	}
	boundary, ok := params["boundary"]
	if !ok || boundary == "" {
		return fmt.Errorf("multipart boundary is missing")
	}
	body, newContentType, err := replaceMultipartModel(raw.Body, boundary, newModel)
	if err != nil {
		return err
	}
	raw.Body = body
	raw.Headers.Set("Content-Type", newContentType)
	return nil
}

// replaceMultipartModel 逐 part 复制表单，仅替换 name==model 的文本 part。
// 缺 model 字段时追加一个，确保上游一定能收到模型名。
func replaceMultipartModel(body []byte, boundary, newModel string) ([]byte, string, error) {
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	mw := multipart.NewWriter(&out)
	found := false
	for {
		part, err := mr.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}
		hdr := make(textproto.MIMEHeader, len(part.Header))
		for k, vv := range part.Header {
			cp := make([]string, len(vv))
			copy(cp, vv)
			hdr[k] = cp
		}
		pw, err := mw.CreatePart(hdr)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}
		if part.FormName() == "model" && part.FileName() == "" {
			if _, err := io.Copy(io.Discard, part); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("discard original model: %w", err)
			}
			_ = part.Close()
			if _, err := io.WriteString(pw, newModel); err != nil {
				return nil, "", fmt.Errorf("write replaced model: %w", err)
			}
			found = true
			continue
		}
		if _, err := io.Copy(pw, part); err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("copy multipart part %q: %w", part.FormName(), err)
		}
		_ = part.Close()
	}
	if !found {
		if err := mw.WriteField("model", newModel); err != nil {
			return nil, "", fmt.Errorf("append missing model: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}
	return out.Bytes(), mw.FormDataContentType(), nil
}
