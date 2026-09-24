package biz

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestHealthGateWebhookEvents(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:health-gate-webhook?mode=memory&_fk=0")
	defer client.Close()

	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read webhook request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- r.Header.Get("X-Axonhub-Event") + " " + string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := WebhookNotifierConfig{
		Targets: []WebhookTarget{
			{Name: "opened", Enabled: true, URL: server.URL, Body: `{{.Event}}|{{.Severity}}|{{.Channel.ID}}|{{.Channel.Name}}|{{.Model.ActualModel}}|{{.Trigger.Type}}|{{.Trigger.Threshold}}|{{.Trigger.ActualCount}}|{{.Trigger.StatusCode}}|{{.Trigger.Reason}}|{{.Trigger.OpenUntil}}|{{.OccurredAt}}`, Headers: []objects.HeaderEntry{{Key: "X-AxonHub-Event", Value: "{{.Event}}"}}},
			{Name: "recovered", Enabled: true, URL: server.URL, Body: `{{.Event}}|{{.Severity}}|{{.Channel.ID}}|{{.Channel.Name}}|{{.Model.ActualModel}}|{{.Trigger.Type}}|{{.Trigger.Threshold}}|{{.Trigger.ActualCount}}|{{.Trigger.StatusCode}}|{{.Trigger.Reason}}|{{.Trigger.OpenUntil}}|{{.OccurredAt}}`, Headers: []objects.HeaderEntry{{Key: "X-AxonHub-Event", Value: "{{.Event}}"}}},
		},
		Subscriptions: []WebhookSubscription{
			{Event: EventChannelHealthGateOpened, TargetNames: []string{"opened"}},
			{Event: EventChannelHealthGateRecovered, TargetNames: []string{"recovered"}},
		},
	}
	notifier := NewWebhookNotifier(newTestSystemServiceWithWebhookConfig(t, client, cfg), httpclient.NewHttpClient())
	at := time.Date(2026, 9, 24, 10, 11, 12, 0, time.UTC)
	event := ChannelHealthGateEvent{
		Transition: HealthGateTransition{
			Key: HealthGateKey{ChannelID: 42, ActualModel: "provider-model"},
			At:  at, OpenUntil: at.Add(5 * time.Minute),
			ConsecutiveFailures: 5, FailureThreshold: 5, ProbeSuccesses: 2, ProbeSuccessThreshold: 2,
			LastStatusCode: 503, LastError: "upstream down",
		},
		ChannelName: "primary", ChannelProvider: "openai", ChannelBaseURL: "https://example.com", ChannelStatus: "enabled",
	}
	notifier.NotifyChannelHealthGateOpened(context.Background(), event)
	select {
	case got := <-received:
		want := "channel.health_gate_opened channel.health_gate_opened|warning|42|primary|provider-model|health_gate_opened|5|5|503|upstream down|2026-09-24T10:16:12Z|2026-09-24T10:11:12Z"
		if got != want {
			t.Fatalf("opened webhook = %q, want %q", got, want)
		}
	default:
		t.Fatal("opened subscription did not receive event")
	}

	event.Transition.OpenUntil = time.Time{}
	notifier.NotifyChannelHealthGateRecovered(context.Background(), event)
	select {
	case got := <-received:
		want := "channel.health_gate_recovered channel.health_gate_recovered|info|42|primary|provider-model|health_gate_recovered|2|2|503|upstream down||2026-09-24T10:11:12Z"
		if got != want {
			t.Fatalf("recovered webhook = %q, want %q", got, want)
		}
	default:
		t.Fatal("recovered subscription did not receive event")
	}
}

func TestHealthGateChannelServiceWebhookBridge(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:health-gate-bridge?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	ch, err := client.Channel.Create().SetType(channel.TypeOpenai).SetName("bridge-primary").SetBaseURL("https://provider.example").SetStatus(channel.StatusEnabled).SetCredentials(objects.ChannelCredentials{APIKey: "key"}).SetSupportedModels([]string{"actual"}).SetDefaultTestModel("actual").Save(ctx)
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read notification: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	config := WebhookNotifierConfig{
		Targets: []WebhookTarget{{Name: "notifications", Enabled: true, URL: server.URL, Body: `{{.Event}}|{{.Channel.Name}}|{{.Channel.Provider}}|{{.Channel.BaseURL}}|{{.Channel.Status}}|{{.Model.ActualModel}}`}},
		Subscriptions: []WebhookSubscription{
			{Event: EventChannelHealthGateOpened, TargetNames: []string{"notifications"}},
			{Event: EventChannelHealthGateRecovered, TargetNames: []string{"notifications"}},
		},
	}
	notifier := NewWebhookNotifier(newTestSystemServiceWithWebhookConfig(t, client, config), httpclient.NewHttpClient())
	svc := &ChannelService{AbstractService: &AbstractService{db: client}, WebhookNotifier: notifier}
	gate := svc.HealthGate()
	key := HealthGateKey{ChannelID: ch.ID, ActualModel: "actual"}
	policy := HealthGateConfig{FailureThreshold: 1, OpenDuration: time.Minute, MaxOpenDuration: time.Minute, ProbeSuccessThreshold: 1, UnstableWindow: time.Minute}
	resolve := healthGateTestResolver(policy)
	ticket, ok := gate.Begin(key, resolve, false, false)
	if !ok {
		t.Fatal("initial attempt rejected")
	}
	gate.Finish(ticket, resolve, HealthGateOutcomeFailure, HealthGateErrorInfo{StatusCode: 503})
	probe, ok := gate.Begin(key, resolve, false, true)
	if !ok {
		t.Fatal("last-resort recovery probe rejected")
	}
	gate.Finish(probe, resolve, HealthGateOutcomeSuccess, HealthGateErrorInfo{})

	seen := make(map[string]bool)
	for range 2 {
		select {
		case body := <-received:
			seen[body] = true
		case <-time.After(5 * time.Second):
			t.Fatal("asynchronous health gate notification did not arrive")
		}
	}
	for _, event := range []string{EventChannelHealthGateOpened, EventChannelHealthGateRecovered} {
		want := event + "|bridge-primary|openai|https://provider.example|enabled|actual"
		if !seen[want] {
			t.Fatalf("missing persisted-channel metadata for %s: %v", event, seen)
		}
	}
}
