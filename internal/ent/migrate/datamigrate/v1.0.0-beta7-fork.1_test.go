package datamigrate_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/migrate/datamigrate"
	"github.com/looplj/axonhub/internal/objects"
)

const (
	forkDirtyUpdatedAt = "2026-08-18 00:13:11.396794292 +0800 CST m=+0.000011483"
	forkCleanUpdatedAt = "2026-08-18 00:13:11.396794292 +0800 CST"
)

func forkUpdatedAtMatches(
	t *testing.T,
	driver *entsql.Driver,
	id int,
	updatedAt string,
) int {
	t.Helper()
	var count int
	err := driver.DB().QueryRowContext(
		context.Background(),
		"SELECT count(*) FROM channels WHERE id = ? AND updated_at = ?",
		id,
		updatedAt,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

func TestV1_0_0_Beta7_Fork_1StripsMonotonicSuffixFromUpdatedAt(t *testing.T) {
	client := enttest.NewEntClient(
		t,
		"sqlite3",
		"file:beta7-fork-1-updated-at?mode=memory&_fk=1",
	)
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())
	channelEntity := client.Channel.Create().
		SetName("dirty-updated-at").
		SetType(channel.TypeOpenai).
		SetCredentials(objects.ChannelCredentials{APIKey: "sk-test"}).
		SetSupportedModels([]string{"test-model"}).
		SetDefaultTestModel("test-model").
		SaveX(ctx)

	driver := client.Driver().(*entsql.Driver)
	_, err := driver.ExecContext(ctx,
		"UPDATE channels SET updated_at = ? WHERE id = ?",
		forkDirtyUpdatedAt,
		channelEntity.ID,
	)
	require.NoError(t, err)
	require.Equal(
		t,
		0,
		forkUpdatedAtMatches(t, driver, channelEntity.ID, forkCleanUpdatedAt),
	)

	require.NoError(t, datamigrate.NewV1_0_0_Beta7_Fork_1().Migrate(ctx, client))
	require.Equal(
		t,
		1,
		forkUpdatedAtMatches(t, driver, channelEntity.ID, forkCleanUpdatedAt),
	)
	require.Equal(
		t,
		0,
		forkUpdatedAtMatches(t, driver, channelEntity.ID, forkDirtyUpdatedAt),
	)
}

func TestV1_0_0_Beta7_Fork_1PreservesCleanUpdatedAt(t *testing.T) {
	client := enttest.NewEntClient(
		t,
		"sqlite3",
		"file:beta7-fork-1-clean-updated-at?mode=memory&_fk=1",
	)
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())
	channelEntity := client.Channel.Create().
		SetName("clean-updated-at").
		SetType(channel.TypeOpenai).
		SetCredentials(objects.ChannelCredentials{APIKey: "sk-test"}).
		SetSupportedModels([]string{"test-model"}).
		SetDefaultTestModel("test-model").
		SaveX(ctx)

	driver := client.Driver().(*entsql.Driver)
	_, err := driver.ExecContext(ctx,
		"UPDATE channels SET updated_at = ? WHERE id = ?",
		forkCleanUpdatedAt,
		channelEntity.ID,
	)
	require.NoError(t, err)

	require.NoError(t, datamigrate.NewV1_0_0_Beta7_Fork_1().Migrate(ctx, client))
	require.Equal(
		t,
		1,
		forkUpdatedAtMatches(t, driver, channelEntity.ID, forkCleanUpdatedAt),
	)
}

type forkRecordingDriver struct {
	dialect     string
	execQueries []string
}

func (d *forkRecordingDriver) Dialect() string { return d.dialect }

func (d *forkRecordingDriver) Close() error { return nil }

func (d *forkRecordingDriver) Tx(context.Context) (dialect.Tx, error) {
	return nil, errors.New("unexpected tx")
}

func (d *forkRecordingDriver) Query(context.Context, string, any, any) error {
	return errors.New("unexpected query")
}

func (d *forkRecordingDriver) Exec(
	_ context.Context,
	query string,
	_ any,
	value any,
) error {
	d.execQueries = append(d.execQueries, query)
	result, ok := value.(*sql.Result)
	if !ok {
		return fmt.Errorf("expected *sql.Result, got %T", value)
	}
	*result = driver.RowsAffected(0)
	return nil
}

func TestV1_0_0_Beta7_Fork_1PostgresSkipsMonotonicCleanup(t *testing.T) {
	drv := &forkRecordingDriver{dialect: dialect.Postgres}
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	ctx := authz.WithTestBypass(context.Background())
	require.NoError(t, datamigrate.NewV1_0_0_Beta7_Fork_1().Migrate(ctx, client))
	require.Empty(t, drv.execQueries)
}
