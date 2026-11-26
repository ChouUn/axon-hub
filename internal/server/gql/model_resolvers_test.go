package gql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/ent/role"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
)

func TestQueryModels_ProjectAPIKeyManagerCanReadModels(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:query-models-project-access?mode=memory&_fk=1")
	defer client.Close()

	setupCtx := authz.WithTestBypass(context.Background())
	project := client.Project.Create().SetName("query-models-project").SaveX(setupCtx)
	roleEntity := client.Role.Create().
		SetName("Developer").
		SetLevel(role.LevelProject).
		SetProjectID(project.ID).
		SetScopes([]string{"write_api_keys"}).
		SaveX(setupCtx)
	userEntity := client.User.Create().
		SetEmail("query-models-user@example.com").
		SetPassword("password").
		SetStatus(user.StatusActivated).
		SaveX(setupCtx)
	client.UserProject.Create().SetUserID(userEntity.ID).SetProjectID(project.ID).SaveX(setupCtx)
	client.UserRole.Create().SetUserID(userEntity.ID).SetRoleID(roleEntity.ID).SaveX(setupCtx)
	client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("query-models-channel").
		SetCredentials(objects.ChannelCredentials{APIKey: "key"}).
		SetSupportedModels([]string{"project-model", "channel-only-model"}).
		SetDefaultTestModel("project-model").
		SetStatus(channel.StatusEnabled).
		SaveX(setupCtx)
	client.Model.Create().SetModelID("project-model").SetName("Project model").SetDeveloper("test").SetIcon("test").SetGroup("test").SetModelCard(&objects.ModelCard{}).SetSettings(&objects.ModelSettings{}).SetStatus(model.StatusEnabled).SaveX(setupCtx)

	userWithEdges := client.User.Query().Where(user.IDEQ(userEntity.ID)).WithRoles().WithProjectUsers().OnlyX(setupCtx)
	ctx := ent.NewContext(context.Background(), client)
	ctx = authz.NewUserContext(ctx, userWithEdges.ID)
	ctx = contexts.WithUser(ctx, userWithEdges)
	ctx = contexts.WithProjectID(ctx, project.ID)

	systemService := biz.NewSystemService(biz.SystemServiceParams{
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
		Ent:         client,
	})
	channelService := biz.NewChannelServiceForTest(client)
	modelService := biz.NewModelService(biz.ModelServiceParams{
		ModelCache: biz.NewModelCacheService(biz.ModelCacheServiceParams{
			Ent: client,
		}),
		ChannelService: channelService,
		SystemService:  systemService,
		Ent:            client,
	})
	resolver := &queryResolver{&Resolver{
		client:         client,
		systemService:  systemService,
		channelService: channelService,
		modelService:   modelService,
	}}

	models, err := resolver.QueryModels(ctx, QueryModelsInput{})
	require.NoError(t, err)
	require.Contains(t, models, &biz.ModelIdentityWithStatus{ID: "project-model", Status: channel.StatusEnabled})
	require.NotContains(t, models, &biz.ModelIdentityWithStatus{ID: "channel-only-model", Status: channel.StatusEnabled})
	_, err = resolver.QueryModels(ctx, QueryModelsInput{IncludeAllChannelModels: new(true)})
	require.Error(t, err)
	adminModels, err := resolver.QueryModels(ent.NewContext(setupCtx, client), QueryModelsInput{IncludeAllChannelModels: new(true)})
	require.NoError(t, err)
	require.Contains(t, adminModels, &biz.ModelIdentityWithStatus{ID: "channel-only-model", Status: channel.StatusEnabled})
	channels, err := resolver.AllChannelSummarys(ctx, nil)
	require.NoError(t, err)
	require.Len(t, channels, 1)

	pluginCtx, err := authz.DelegatePlugin(ctx, 1, []string{"write_api_keys"})
	require.NoError(t, err)
	pluginModels, err := resolver.QueryModels(pluginCtx, QueryModelsInput{})
	require.NoError(t, err)
	require.Equal(t, models, pluginModels)
	pluginChannels, err := resolver.AllChannelSummarys(pluginCtx, nil)
	require.NoError(t, err)
	require.Len(t, pluginChannels, 1)
	require.Equal(t, channels[0].ID, pluginChannels[0].ID)
	revoked, err := authz.DelegatePlugin(ctx, 2, nil)
	require.NoError(t, err)
	_, err = resolver.QueryModels(revoked, QueryModelsInput{})
	require.Error(t, err)
	_, err = resolver.AllChannelSummarys(revoked, nil)
	require.Error(t, err)

	ctxWithoutProject := ent.NewContext(context.Background(), client)
	ctxWithoutProject = authz.NewUserContext(ctxWithoutProject, userWithEdges.ID)
	ctxWithoutProject = contexts.WithUser(ctxWithoutProject, userWithEdges)
	_, err = resolver.QueryModels(ctxWithoutProject, QueryModelsInput{})
	require.Error(t, err)
}

func TestUpdateModelExpectedVersionGraphQL(t *testing.T) {
	db := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	t.Cleanup(func() { _ = db.Close() })
	srv := NewGraphqlHandlers(Dependencies{Ent: db, ModelService: biz.NewModelService(biz.ModelServiceParams{Ent: db})})
	api := client.New(srv.Graphql, func(r *client.Request) {
		r.HTTP = r.HTTP.WithContext(authz.WithTestBypass(r.HTTP.Context()))
	})
	var created struct {
		CreateModel struct{ ID, EditVersion string }
	}
	require.NoError(t, api.Post(`mutation {
		createModel(input: {
			modelID: "versioned", name: "Original", developer: "dev", icon: "dev", group: "dev",
			modelCard: {}, settings: {associations: []}
		}) { id editVersion }
	}`, &created))
	require.NotEmpty(t, created.CreateModel.EditVersion)
	const update = `mutation Update($id: ID!, $version: String!, $name: String!) {
		updateModel(id: $id, input: {name: $name}, expectedVersion: $version) { name }
	}`
	var updated struct {
		UpdateModel struct{ Name string }
	}
	id := client.Var("id", created.CreateModel.ID)
	version := client.Var("version", created.CreateModel.EditVersion)
	require.NoError(t, api.Post(update, &updated, id, version, client.Var("name", "Updated")))
	require.Equal(t, "Updated", updated.UpdateModel.Name)
	require.ErrorContains(t, api.Post(update, &updated, id, version, client.Var("name", "Stale")), "model changed since it was opened")
	require.Equal(t, "Updated", db.Model.Query().OnlyX(authz.WithTestBypass(t.Context())).Name)
}
