package balancer

import (
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/charmbracelet/log"
)

// CircuitState 熔断器状态。
type CircuitState int

const (
	StateClosed   CircuitState = iota // 正常通行。
	StateOpen                         // 熔断中,拒绝所有请求。
	StateHalfOpen                     // 半开,允许单个试探请求。

	baseCooldownSeconds = 60  // 首次熔断的基础冷却秒数。
	maxCooldownSeconds  = 600 // 指数退避的冷却上限秒数。
)

// circuitEntry 单个熔断器条目。
type circuitEntry struct {
	State               CircuitState
	ConsecutiveFailures int64
	LastFailureTime     time.Time
	TripCount           int // 累计熔断触发次数(用于指数退避)。
	mu                  sync.Mutex
}

// globalBreaker 全局熔断器存储,key 为 channelID:keyID:modelName。
var globalBreaker sync.Map

// circuitKey 生成熔断器键。
func circuitKey(channelID, keyID int, modelName string) string {
	return fmt.Sprintf("%d:%d:%s", channelID, keyID, modelName)
}

// getOrCreateEntry 获取或创建熔断器条目。
func getOrCreateEntry(key string) *circuitEntry {
	if v, ok := globalBreaker.Load(key); ok {
		return v.(*circuitEntry)
	}
	entry := &circuitEntry{State: StateClosed}
	actual, _ := globalBreaker.LoadOrStore(key, entry)
	return actual.(*circuitEntry)
}

// getThreshold 获取熔断阈值配置。
func getThreshold() int64 {
	v, err := op.SettingGetInt(model.SettingKeyCircuitBreakerThreshold)
	if err != nil || v <= 0 {
		return 5
	}
	return int64(v)
}

// GetCooldown 获取当前冷却时间(带指数退避)。
func GetCooldown(tripCount int) time.Duration {
	cooldown := baseCooldownSeconds
	if tripCount > 1 {
		shift := tripCount - 1
		if shift > 20 { // 防止溢出。
			shift = 20
		}
		cooldown = baseCooldownSeconds << shift
	}
	if cooldown > maxCooldownSeconds {
		cooldown = maxCooldownSeconds
	}
	return time.Duration(cooldown) * time.Second
}

// IsTripped 检查渠道是否处于熔断状态;tripped 为 true 表示应跳过,remaining 为剩余冷却时间。
func IsTripped(channelID, keyID int, modelName string) (tripped bool, remaining time.Duration) {
	key := circuitKey(channelID, keyID, modelName)
	v, ok := globalBreaker.Load(key)
	if !ok {
		return false, 0 // 无记录视为 Closed。
	}
	entry := v.(*circuitEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	switch entry.State {
	case StateClosed:
		return false, 0

	case StateOpen:
		cooldown := GetCooldown(entry.TripCount)
		elapsed := time.Since(entry.LastFailureTime)
		if elapsed >= cooldown {
			entry.State = StateHalfOpen
			log.Infof("circuit breaker [%s] Open -> HalfOpen (cooldown %v elapsed)", key, cooldown)
			return false, 0
		}
		return true, cooldown - elapsed

	case StateHalfOpen:
		return true, 0

	default:
		return false, 0
	}
}

// RecordSuccess 记录成功并重置熔断器状态。
func RecordSuccess(channelID, keyID int, modelName string) {
	key := circuitKey(channelID, keyID, modelName)
	v, ok := globalBreaker.Load(key)
	if !ok {
		return
	}
	entry := v.(*circuitEntry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.State == StateHalfOpen {
		log.Infof("circuit breaker [%s] HalfOpen -> Closed (probe succeeded)", key)
	}
	entry.State = StateClosed
	entry.ConsecutiveFailures = 0
	entry.TripCount = 0
}

// RecordFailure 记录失败,达到阈值时触发熔断。
func RecordFailure(channelID, keyID int, modelName string) {
	key := circuitKey(channelID, keyID, modelName)
	entry := getOrCreateEntry(key)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.LastFailureTime = time.Now()

	switch entry.State {
	case StateClosed:
		entry.ConsecutiveFailures++
		threshold := getThreshold()
		if entry.ConsecutiveFailures >= threshold {
			entry.State = StateOpen
			entry.TripCount++
			log.Warnf("circuit breaker [%s] Closed -> Open (failures=%d >= threshold=%d, tripCount=%d, cooldown=%v)",
				key, entry.ConsecutiveFailures, threshold, entry.TripCount, GetCooldown(entry.TripCount))
		}

	case StateHalfOpen:
		entry.State = StateOpen
		entry.TripCount++
		entry.ConsecutiveFailures = 0
		log.Warnf("circuit breaker [%s] HalfOpen -> Open (probe failed, tripCount=%d, cooldown=%v)",
			key, entry.TripCount, GetCooldown(entry.TripCount))

	case StateOpen:
		// Open 状态下正常不应收到失败记录,仍更新失败时间以保证安全。
	}
}
