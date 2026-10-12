package sidecar

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// Manager 管 Zen 侧车子进程：启动、健康探活、崩溃重启、退出时带走。
type Manager struct {
	binary    string
	apiAddr   string
	config    string
	cmd       *exec.Cmd
	mu        sync.Mutex
	stopCh    chan struct{}
	stopped   bool
	restartIn time.Duration
}

func New(binary, apiAddr, config string) *Manager {
	return &Manager{
		binary:    binary,
		apiAddr:   apiAddr,
		config:    config,
		restartIn: 5 * time.Second,
	}
}

// 全局实例，供 WebUI 运行时启停；仅在 start 流程拉起成功后设置。
var (
	globalMu sync.Mutex
	global   *Manager
)

// SetGlobal 登记当前运行的侧车实例；nil 表示已停止。
func SetGlobal(m *Manager) {
	globalMu.Lock()
	defer globalMu.Unlock()
	global = m
}

// Global 返回当前侧车实例，未运行时返回 nil。
func Global() *Manager {
	globalMu.Lock()
	defer globalMu.Unlock()
	return global
}

// Running 报告侧车进程是否存活。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil && !m.stopped
}

// ensureConfig 侧车配置不存在时按匿名免费优先模板生成，local key 自动随机。
func (m *Manager) ensureConfig(adminAddr string) error {
	if _, err := os.Stat(m.config); err == nil {
		return nil
	}
	key := make([]byte, 24)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate sidecar local key: %w", err)
	}
	cfg := map[string]any{
		"listen":      m.apiAddr,
		"server_keys": []string{hex.EncodeToString(key)},
		"zen_keys":    []string{},
		"go_keys":     []string{},
		"anonymous":   true,
		"proxies":     []string{"direct"},
		"upstream": map[string]string{
			"zen": "https://opencode.ai/zen",
			"go":  "https://opencode.ai/zen/go",
		},
		"retry":   map[string]any{"max_attempts": 3, "timeout_seconds": 300},
		"models":  map[string]any{"refresh_seconds": 300, "protocols": map[string]any{}},
		"logging": map[string]any{"level": "info", "ring_size": 2000},
		"webui": map[string]any{
			"enabled": true, "listen": adminAddr,
			"username": "admin", "password": "change-this-admin-password",
			"session_ttl_minutes": 720,
		},
		"prefer":    "zen",
		"reasoning": map[string]any{"effort": "high", "effort_by_model": map[string]any{}},
	}
	if err := os.MkdirAll(filepath.Dir(m.config), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.config, data, 0600); err != nil {
		return err
	}
	log.Infof("[octopus] sidecar config created: %s (anonymous first, local key randomized)", m.config)
	return nil
}

// Start 拉起侧车并进入探活循环；二进制缺失时只告警不阻断主服务。
func (m *Manager) Start(adminAddr string) error {
	if _, err := os.Stat(m.binary); err != nil {
		log.Warnf("[octopus] sidecar binary not found: %s, skip sidecar", m.binary)
		return nil
	}
	if err := m.ensureConfig(adminAddr); err != nil {
		return err
	}
	m.stopCh = make(chan struct{})
	go m.loop()
	return nil
}

func (m *Manager) loop() {
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}
		if err := m.runOnce(); err != nil {
			log.Warnf("[octopus] sidecar exited: %v, restart in %v", err, m.restartIn)
		}
		select {
		case <-m.stopCh:
			return
		case <-time.After(m.restartIn):
		}
	}
}

func (m *Manager) runOnce() error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil
	}
	m.cmd = exec.Command(m.binary, "-config", m.config)
	m.cmd.Stdout = logWriter{}
	m.cmd.Stderr = logWriter{}
	if err := m.cmd.Start(); err != nil {
		m.mu.Unlock()
		return err
	}
	log.Infof("[octopus] sidecar started: %s pid=%d", m.binary, m.cmd.Process.Pid)
	cmd := m.cmd
	m.mu.Unlock()

	errCh := make(chan error, 1)
	go func() { errCh <- cmd.Wait() }()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	// 启动宽限：模型目录刷新完成前 healthz 可能非 ready，不计入失败。
	graceUntil := time.Now().Add(30 * time.Second)
	fails := 0
	for {
		select {
		case <-m.stopCh:
			m.mu.Lock()
			m.stopped = true
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			m.mu.Unlock()
			<-errCh
			return nil
		case err := <-errCh:
			return err
		case <-ticker.C:
			if !m.healthy() {
				if time.Now().Before(graceUntil) {
					continue
				}
				fails++
				// 连续失败才告警，单次抖动不打扰。
				if fails >= 3 {
					log.Warnf("[octopus] sidecar unhealthy: %s (%d consecutive)", m.apiAddr, fails)
				}
			} else {
				fails = 0
			}
		}
	}
}

func (m *Manager) healthy() bool {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + m.apiAddr + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Close 停止探活并杀掉侧车，供 shutdown.Register 调用。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopCh != nil {
		select {
		case <-m.stopCh:
		default:
			close(m.stopCh)
		}
	}
	m.stopped = true
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		log.Infof("[octopus] sidecar stopped")
	}
	return nil
}

// logWriter 把侧车 stdout/stderr 接到主日志。
type logWriter struct{}

func (logWriter) Write(p []byte) (int, error) {
	log.Infof("[sidecar] %s", string(p))
	return len(p), nil
}
