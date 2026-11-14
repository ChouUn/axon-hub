package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
)

type passThroughClosePauseMiddleware struct {
	pipeline.DummyMiddleware
	paused      chan struct{}
	closing     chan struct{}
	release     chan struct{}
	nextStopped chan struct{}
	releaseOnce sync.Once
}

func (m *passThroughClosePauseMiddleware) OnOutboundLlmStream(_ context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	return &passThroughClosePauseStream{Stream: stream, control: m}, nil
}

type passThroughClosePauseStream struct {
	streams.Stream[*llm.Response]
	control  *passThroughClosePauseMiddleware
	once     sync.Once
	stopOnce sync.Once
	paused   bool
	output   bool
}

func (s *passThroughClosePauseStream) Next() bool {
	if s.output && !s.paused {
		s.paused = true
		close(s.control.paused)
		<-s.control.release
	}
	more := s.Stream.Next()
	if !more {
		s.stopOnce.Do(func() { close(s.control.nextStopped) })
	}
	return more
}

func (s *passThroughClosePauseStream) Current() *llm.Response {
	response := s.Stream.Current()
	for _, choice := range response.Choices {
		if choice.Delta != nil && choice.Delta.Content.Content != nil && *choice.Delta.Content.Content != "" {
			s.output = true
		}
	}
	return response
}

func (s *passThroughClosePauseStream) Close() error {
	s.once.Do(func() { close(s.control.closing) })
	return s.Stream.Close()
}

func TestPassThroughCloseQuiescesConvertedDrain(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		t.Run(fmt.Sprintf("delivered=%t", delivered), func(t *testing.T) {
			control := &passThroughClosePauseMiddleware{paused: make(chan struct{}), closing: make(chan struct{}), release: make(chan struct{}), nextStopped: make(chan struct{})}
			var state *PersistenceState
			process := anthropicPreReadPipeline(t, &mockExecutor{streamEvents: anthropicPreReadEvents(0, false)}, func(current *PersistenceState) pipeline.Middleware {
				state = current
				return control
			})
			downstream, cancel := context.WithCancel(t.Context())
			defer cancel()
			owner, ctx := newStreamLifecycle(downstream)
			defer owner.Close()
			defer control.releaseOnce.Do(func() { close(control.release) })
			result, err := process(ctx, anthropicPreReadRequest())
			require.NoError(t, err)
			select {
			case <-control.paused:
			case <-time.After(5 * time.Second):
				t.Fatal("converted drain did not pause after output")
			}
			if delivered {
				owner.AcknowledgeDelivery()
			} else {
				cancel()
			}
			done := make(chan error, 1)
			go func() { done <- result.EventStream.Close() }()
			select {
			case <-control.closing:
			case <-time.After(5 * time.Second):
				t.Fatal("pass-through close did not stop the converted consumer")
			}
			owner.WaitForFinalization()
			select {
			case err := <-done:
				t.Fatalf("close returned while converted drain was paused: %v", err)
			default:
			}
			control.releaseOnce.Do(func() { close(control.release) })
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("converted drain did not exit after close and lifecycle cancellation")
			}
			select {
			case <-state.rawStreamDrainDone:
			default:
				t.Fatal("close did not wait for converted drain completion")
			}
			select {
			case <-control.nextStopped:
			default:
				t.Fatal("converted stream Next did not observe closure")
			}
		})
	}
}
