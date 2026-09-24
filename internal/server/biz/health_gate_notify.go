package biz

import (
	"context"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/log"
)

// notifyHealthGateTransition never runs under the gate's observation or state locks.
func (svc *ChannelService) notifyHealthGateTransition(transition HealthGateTransition) {
	if svc.WebhookNotifier == nil || svc.AbstractService == nil || svc.db == nil {
		return
	}

	go func() {
		ctx := authz.WithSystemBypass(context.WithoutCancel(context.Background()), "health-gate-webhook")
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error(ctx, "health gate webhook notification panicked", log.Any("panic", recovered))
			}
		}()

		channel, err := svc.db.Channel.Get(ctx, transition.Key.ChannelID)
		if err != nil {
			log.Warn(ctx, "failed to read channel for health gate webhook",
				log.Int("channel_id", transition.Key.ChannelID), log.Cause(err))
			return
		}
		event := ChannelHealthGateEvent{
			Transition:      transition,
			ChannelName:     channel.Name,
			ChannelProvider: channel.Type.String(),
			ChannelBaseURL:  channel.BaseURL,
			ChannelStatus:   channel.Status.String(),
		}
		switch transition.Kind {
		case "opened":
			svc.WebhookNotifier.NotifyChannelHealthGateOpened(ctx, event)
		case "recovered":
			svc.WebhookNotifier.NotifyChannelHealthGateRecovered(ctx, event)
		}
	}()
}
