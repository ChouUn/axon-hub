// 职责: 验证模型名推理档位后缀的拆分规则。
// 关联文件:
//
//	llm/reasoning.go

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitReasoningEffortModelSuffix(t *testing.T) {
	for _, tc := range []struct {
		model  string
		base   string
		effort string
		ok     bool
	}{
		{model: "gemini-4-hydrogen-xhigh", base: "gemini-4-hydrogen", effort: "xhigh", ok: true},
		{model: "gpt-5.4-HIGH", base: "gpt-5.4", effort: "high", ok: true},
		{model: "Gemini-4-Argon-Low", base: "Gemini-4-Argon", effort: "low", ok: true},
		{model: "gpt-5.4-max", base: "gpt-5.4", effort: "max", ok: true},
		{model: "qwen3-max"},
		{model: "qwen/qwen3-max"},
		{model: "gpt-5.4-minimal"},
		{model: "gpt-5.4-none"},
		{model: "gpt-5.4-ultra"},
		{model: "-high"},
		{model: "m-high-"},
		{model: "gpt-5.4"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			base, effort, ok := SplitReasoningEffortModelSuffix(tc.model)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.base, base)
			require.Equal(t, tc.effort, effort)
		})
	}
}
