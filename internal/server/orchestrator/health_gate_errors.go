package orchestrator

import (
	"errors"

	"github.com/looplj/axonhub/llm"
)

var errSkipCandidateByHealthGate = errors.New("candidate skipped by health gate")

func healthGateUnavailableError() *llm.ResponseError {
	return &llm.ResponseError{
		StatusCode: 503,
		Detail: llm.ErrorDetail{
			Message: "Service temporarily unavailable, please retry later.",
			Type:    "service_unavailable",
			Code:    "service_unavailable",
		},
	}
}
