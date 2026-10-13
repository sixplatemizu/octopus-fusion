package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/sidecar"
	"github.com/bestruirui/octopus/internal/utils/shutdown"
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
		).
		AddRoute(
			router.NewRoute("/start", http.MethodPost).
				Handle(startSidecar),
		).
		AddRoute(
			router.NewRoute("/stop", http.MethodPost).
				Handle(stopSidecar),
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return 0, nil, err
	}
	return r.StatusCode, body, nil
}

func getSidecarStatus(c *gin.Context) {
	// running 是当前事实：进程存活且探活通过才为 true。
	// 侧车默认关闭：--o2a 或面板启动仅影响本轮运行，重启后默认关闭。
	out := map[string]any{
		"running": false,
	}
	if sidecar.Global() == nil || !sidecar.Global().Healthy() {
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

// sidecarLifecycleMu 串行化运行时启停：检查-拉起-登记非原子，加锁防连点拉起多个进程。
var sidecarLifecycleMu sync.Mutex

// startSidecar 运行时拉起侧车：仅影响本轮运行，退出时由 shutdown 带走，不写配置文件。
func startSidecar(c *gin.Context) {
	sidecarLifecycleMu.Lock()
	defer sidecarLifecycleMu.Unlock()
	if sidecar.Global() != nil && sidecar.Global().Running() {
		resp.Error(c, http.StatusBadRequest, "sidecar already running")
		return
	}
	sc := sidecar.New(conf.AppConfig.Sidecar.Binary, conf.AppConfig.Sidecar.APIAddr, conf.AppConfig.Sidecar.Config)
	if err := sc.Start(conf.AppConfig.Sidecar.AdminAddr); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	shutdown.Register(sc.Close)
	sidecar.SetGlobal(sc)
	resp.Success(c, map[string]any{"running": true})
}

// stopSidecar 运行时停止侧车：仅影响本轮运行；默认关闭，下次启动仍需 --o2a 或手动启动。
func stopSidecar(c *gin.Context) {
	sidecarLifecycleMu.Lock()
	defer sidecarLifecycleMu.Unlock()
	m := sidecar.Global()
	if m == nil || !m.Running() {
		resp.Error(c, http.StatusBadRequest, "sidecar not running")
		return
	}
	if err := m.Close(); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	sidecar.SetGlobal(nil)
	resp.Success(c, map[string]any{"running": false})
}
