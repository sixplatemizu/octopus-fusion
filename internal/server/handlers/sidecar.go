package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/sidecar").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/status", http.MethodGet).
				Handle(getSidecarStatus),
		).
		AddRoute(
			router.NewRoute("/config", http.MethodGet).
				Handle(getSidecarConfig),
		).
		AddRoute(
			router.NewRoute("/config", http.MethodPost).
				Use(middleware.RequireJSON()).
				Handle(setSidecarConfig),
		)
}

// sidecarHTTP 直调侧车 API，超时 5 秒。
func sidecarHTTP(method, path string) (int, []byte, error) {
	client := http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(method, "http://"+conf.AppConfig.Sidecar.APIAddr+path, nil)
	if err != nil {
		return 0, nil, err
	}
	r, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer r.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return r.StatusCode, buf, nil
}

func getSidecarStatus(c *gin.Context) {
	out := map[string]any{
		"enabled": conf.AppConfig.Sidecar.Enabled,
		"running": false,
	}
	if !conf.AppConfig.Sidecar.Enabled {
		resp.Success(c, out)
		return
	}
	code, body, err := sidecarHTTP(http.MethodGet, "/healthz")
	if err != nil || code != http.StatusOK {
		out["error"] = "sidecar unreachable"
		if err != nil {
			out["error"] = err.Error()
		}
		resp.Success(c, out)
		return
	}
	var health map[string]any
	if err := json.Unmarshal(body, &health); err == nil {
		out["health"] = health
	}
	out["running"] = true
	resp.Success(c, out)
}

// sanitizeSidecarConfig 读配置时脱敏：server_keys 与 webui.password 不回显。
func sanitizeSidecarConfig(raw map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range raw {
		out[k] = v
	}
	if keys, ok := out["server_keys"].([]any); ok {
		masked := make([]string, len(keys))
		for i := range keys {
			masked[i] = "***"
		}
		out["server_keys"] = masked
	}
	if webui, ok := out["webui"].(map[string]any); ok {
		w := map[string]any{}
		for k, v := range webui {
			w[k] = v
		}
		if _, ok := w["password"]; ok {
			w["password"] = "***"
		}
		out["webui"] = w
	}
	return out
}

func readSidecarConfigFile() (map[string]any, error) {
	data, err := os.ReadFile(conf.AppConfig.Sidecar.Config)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func getSidecarConfig(c *gin.Context) {
	raw, err := readSidecarConfigFile()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, sanitizeSidecarConfig(raw))
}

func setSidecarConfig(c *gin.Context) {
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	raw, err := readSidecarConfigFile()
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 允许改的键白名单；listen 类须重启生效，由前端标注。
	allowed := map[string]bool{
		"anonymous": true, "zen_keys": true, "go_keys": true,
		"prefer": true, "retry": true, "reasoning": true,
		"proxies": true, "proxyfile": true, "models": true,
		"logging": true, "performance": true,
	}
	for k, v := range patch {
		if !allowed[k] {
			resp.Error(c, http.StatusBadRequest, "field not editable here: "+k)
			return
		}
		// "***" 表示未改动，保留原值，避免脱敏回写覆盖真 key。
		if s, ok := v.(string); ok && s == "***" {
			continue
		}
		raw[k] = v
	}
	// zen_keys/go_keys 非空字符串数组校验。
	for _, k := range []string{"zen_keys", "go_keys"} {
		if v, ok := raw[k]; ok && v != nil {
			arr, ok := v.([]any)
			if !ok {
				resp.Error(c, http.StatusBadRequest, k+" must be string array")
				return
			}
			for _, e := range arr {
				s, ok := e.(string)
				if !ok || strings.TrimSpace(s) == "" {
					resp.Error(c, http.StatusBadRequest, k+" contains empty key")
					return
				}
			}
		}
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(conf.AppConfig.Sidecar.Config, data, 0600); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, sanitizeSidecarConfig(raw))
}
