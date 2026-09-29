package orchestrator

import (
	"context"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
)

// applyForceOneHourPromptCache extends existing Anthropic breakpoints after body
// pass-through but before channel body overrides, which retain final precedence.
func applyForceOneHourPromptCache(outbound *PersistentOutboundTransformer) pipeline.Middleware {
	return pipeline.OnRawRequest("force-one-hour-prompt-cache", func(ctx context.Context, request *httpclient.Request) (*httpclient.Request, error) {
		channel := outbound.GetCurrentChannel()
		if channel == nil || channel.Settings == nil || !channel.Settings.ForceOneHourPromptCache ||
			request == nil || request.APIFormat != string(llm.APIFormatAnthropicMessage) {
			return request, nil
		}

		if !gjson.ValidBytes(request.Body) {
			log.Warn(ctx, "skipping one-hour prompt cache for invalid JSON request body",
				log.String("channel", channel.Name), log.Int("channel_id", channel.ID))
			return request, nil
		}

		var paths []string
		biz.WalkAnthropicCacheControls(request.Body, func(path string, control gjson.Result) {
			if control.Get("type").String() == "ephemeral" && control.Get("ttl").String() != "1h" {
				paths = append(paths, path+".ttl")
			}
		})

		if len(paths) == 0 {
			return request, nil
		}

		// sjson.SetBytes may write into the input's spare capacity. A previous attempt
		// or the inbound raw request can share this backing array, so copy before patching.
		body := append([]byte(nil), request.Body...)
		for _, path := range paths {
			var err error
			body, err = sjson.SetBytes(body, path, "1h")
			if err != nil {
				log.Warn(ctx, "failed to set one-hour prompt cache TTL",
					log.String("channel", channel.Name), log.Int("channel_id", channel.ID), log.Cause(err))
				return request, nil
			}
		}
		request.Body = body
		return request, nil
	})
}

