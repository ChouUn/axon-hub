package orchestrator

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer/openai"
	"github.com/looplj/axonhub/llm/transformer/openai/responses"
	"github.com/looplj/axonhub/llm/transformer/shared"
)

func TestChatCompletionOrchestrator_RegisteredAssociationAdmission(t *testing.T) {
	ctx, client := setupTest(t)
	project := createTestProject(t, ctx, client)
	createTestChannel(t, ctx, client).Update().SetStatus(channel.StatusEnabled).SaveX(ctx)
	ctx = contexts.WithProjectID(ctx, project.ID)

	user, err := client.User.Create().SetEmail("admission@example.com").SetPassword("password").Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().
		SetName("Mapped admission").SetKey("sk-admission").SetProjectID(project.ID).SetUserID(user.ID).
		SetProfiles(&objects.APIKeyProfiles{ActiveProfile: "default", Profiles: []objects.APIKeyProfile{{
			Name: "default", ModelIDs: []string{"alias", "gpt-4"},
			ModelMappings: []objects.ModelMapping{{From: "alias", To: "gpt-4"}},
		}}}).Save(ctx)
	require.NoError(t, err)
	ctx = contexts.WithAPIKey(ctx, key)

	executor := &mockExecutor{response: &httpclient.Response{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       buildMockOpenAIResponse("chatcmpl-admission", "gpt-4", "allowed", 1, 1),
	}}
	processor := newTestOrchestrator(t, nil, client, executor)
	processor.ChannelService = newTestChannelServiceForChannels(client)
	processor.channelSelector = NewDefaultSelector(processor.ChannelService, newTestModelService(client), processor.SystemService)

	assertRejected := func() {
		t.Helper()
		before := executor.requestCalls.Load()
		for _, requested := range []string{"gpt-4", "alias"} {
			_, err := processor.Process(ctx, buildTestRequest(requested, "hello", false))
			require.ErrorIs(t, err, biz.ErrInvalidModel)
		}
		require.Equal(t, before, executor.requestCalls.Load(), "rejected models must not reach an upstream")
	}

	// A supported channel ID alone, including an API-key mapping target, is insufficient.
	assertRejected()
	registered := createAssociatedTestModel(t, ctx, client, "gpt-4")
	for _, status := range []model.Status{model.StatusDisabled, model.StatusArchived} {
		_, err := client.Model.UpdateOneID(registered.ID).SetStatus(status).Save(ctx)
		require.NoError(t, err)
		assertRejected()
	}

	_, err = client.Model.UpdateOneID(registered.ID).SetStatus(model.StatusEnabled).
		SetSettings(&objects.ModelSettings{}).Save(ctx)
	require.NoError(t, err)
	assertRejected()

	_, err = client.Model.UpdateOneID(registered.ID).SetSettings(registered.Settings).Save(ctx)
	require.NoError(t, err)
	for _, requested := range []string{"gpt-4", "alias"} {
		result, err := processor.Process(ctx, buildTestRequest(requested, "hello", false))
		require.NoError(t, err)
		var response openai.Response
		require.NoError(t, json.Unmarshal(result.ChatCompletion.Body, &response))
		require.Equal(t, requested, response.Model)
		require.Equal(t, "allowed", *response.Choices[0].Message.Content.Content)
	}
	require.EqualValues(t, 2, executor.requestCalls.Load())

	// Previously resolved associations must not admit a model after it is disabled.
	_, err = client.Model.UpdateOneID(registered.ID).SetStatus(model.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	assertRejected()
}

func TestChatCompletionOrchestrator_ResponsesContinuationRechecksRegistration(t *testing.T) {
	ctx, client := setupTest(t)
	createTestChannel(t, ctx, client).Update().SetStatus(channel.StatusEnabled).SaveX(ctx)
	registered := createAssociatedTestModel(t, ctx, client, "gpt-4")
	ctx = shared.WithSessionScope(shared.WithResponsesAPI(ctx), "admission-test")
	executor := &mockExecutor{}
	processor := newTestOrchestrator(t, nil, client, executor)
	processor.ChannelService = newTestChannelServiceForChannels(client)
	processor.Inbound = responses.NewInboundTransformer()
	processor.channelSelector = NewDefaultSelector(processor.ChannelService, newTestModelService(client), processor.SystemService)
	processor.responsesSessions = newResponsesSessionStore()
	processor.responsesSessions.record(ctx,
		[]byte(`{"model":"gpt-4","input":"hello"}`),
		[]byte(`{"id":"resp_previous","status":"completed","output":[]}`))
	_, err := client.Model.UpdateOneID(registered.ID).SetStatus(model.StatusArchived).Save(ctx)
	require.NoError(t, err)

	_, err = processor.Process(ctx, &httpclient.Request{
		Method: http.MethodPost, Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body: []byte(`{"model":"gpt-4","previous_response_id":"resp_previous","input":"continue"}`),
	})
	require.ErrorIs(t, err, biz.ErrInvalidModel)
	require.Zero(t, executor.requestCalls.Load())
}
