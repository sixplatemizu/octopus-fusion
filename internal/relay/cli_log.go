package relay

import (
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/charmbracelet/log"
	"github.com/looplj/axonhub/llm"
)

// CLI 调用日志：对齐旧版 v0.9.28 在控制台持续输出调用记录的习惯。
// 成功走 Info，失败/取消走 Warn；默认 info 级别两者都显示，warn 级别只显示失败。

// logForwarding 记录本轮选路，对应旧版 forwarding to channel 行。
// effort 为客户端声明的思考强度（空表示未声明），供判断透传链路是否携带思考参数。
func logForwarding(requestID uint64, modelName string, mode model.GroupMode, channelID int, channelName, targetModel string, round int, streaming bool, effort string) {
	log.Infof("[octopus] request #%d model=%s mode=%s forwarding to channel=%d(%s) model=%s (round %d, stream=%t, effort=%s)",
		requestID, modelName, mode, channelID, channelName, targetModel, round, streaming, effort)
}

// logRoundFailed 记录一轮上游失败（后续重试或冷却），对应旧版 channel failed 行。
func logRoundFailed(requestID uint64, round, channelID int, channelName, targetModel, errText string) {
	log.Warnf("[octopus] request #%d round %d channel=%d(%s) model=%s failed: %s",
		requestID, round, channelID, channelName, targetModel, errText)
}

// logReject 记录尚未登记请求状态即被拒绝的调用。
func logReject(modelName string, err error) {
	log.Warnf("[octopus] reject model=%s error=%s", modelName, err)
}

// logRelayDone 记录请求终态，对应旧版 relay complete 行。
// effort 为客户端声明的思考强度；reasoningTokens 为上游实际返回的思考 token 数（-1 表示未知），
// 两者对照即可判断思考强度是否真实生效：声明了 xhigh 但 reasoning_tokens=0 就是没生效。
func logRelayDone(requestID uint64, modelName string, channelID int, channelName, targetModel string, success bool, status Status, startedAt time.Time, usage *llm.Usage, errText string, rounds int, effort string) {
	metrics := usageMetrics(targetModel, usage)
	duration := time.Since(startedAt).Milliseconds()
	total := metrics.InputCost + metrics.OutputCost
	var in, out, reasoning int64
	reasoning = -1
	if usage != nil {
		in = usage.PromptTokens
		out = usage.CompletionTokens
		if usage.CompletionTokensDetails != nil {
			reasoning = usage.CompletionTokensDetails.ReasoningTokens
		}
	}
	if success {
		log.Infof("[octopus] relay complete: id=%d model=%s channel=%d(%s) actual_model=%s success=true duration=%dms input_token=%d output_token=%d reasoning_tokens=%d input_cost=%f output_cost=%f total_cost=%f rounds=%d effort=%s",
			requestID, modelName, channelID, channelName, targetModel, duration, in, out, reasoning, metrics.InputCost, metrics.OutputCost, total, rounds, effort)
		return
	}
	log.Warnf("[octopus] relay complete: id=%d model=%s channel=%d(%s) actual_model=%s success=false status=%s duration=%dms input_token=%d output_token=%d reasoning_tokens=%d rounds=%d effort=%s error=%s",
		requestID, modelName, channelID, channelName, targetModel, status, duration, in, out, reasoning, rounds, effort, errText)
}
