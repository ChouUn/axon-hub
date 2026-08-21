package datamigrate

import (
	"context"
	"database/sql"
	"fmt"

	"entgo.io/ent/dialect"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/log"
)

// V1_0_0_Beta7_Fork_1 repairs channel timestamps written by beta7 model price saves.
// The fork-specific pre-release sits between beta7 and beta8 without claiming an
// upstream migration version.
type V1_0_0_Beta7_Fork_1 struct{}

// NewV1_0_0_Beta7_Fork_1 creates the fork-specific beta7 data migrator.
func NewV1_0_0_Beta7_Fork_1() DataMigrator {
	return &V1_0_0_Beta7_Fork_1{}
}

// Version returns the migration version.
func (v *V1_0_0_Beta7_Fork_1) Version() string {
	return "v1.0.0-beta7-fork.1"
}

// Migrate removes the monotonic suffix persisted by the beta7 price writer.
func (v *V1_0_0_Beta7_Fork_1) Migrate(ctx context.Context, client *ent.Client) error {
	ctx = authz.WithSystemBypass(ctx, "database-migrate")

	if client.Driver().Dialect() == dialect.Postgres {
		// PostgreSQL stores updated_at as typed timestamptz, so the SQLite-only
		// monotonic suffix cannot exist and must not be treated as text.
		log.Info(ctx,
			"Skipping channel updated_at monotonic cleanup on PostgreSQL")
		return nil
	}
	if client.Driver().Dialect() != dialect.SQLite {
		log.Info(ctx,
			"Unsupported dialect, skipping channel updated_at "+
				"monotonic cleanup",
			log.String("dialect", client.Driver().Dialect()))
		return nil
	}

	const stmt = "UPDATE channels SET updated_at = substr(updated_at, 1, " +
		"instr(updated_at, ' m=') - 1) " +
		"WHERE updated_at LIKE '% m=%'"
	var result sql.Result
	if err := client.Driver().Exec(ctx, stmt, []any{}, &result); err != nil {
		return fmt.Errorf(
			"failed to strip monotonic suffix from "+
				"channels.updated_at: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		log.Warn(ctx,
			"failed to read affected rows after updated_at cleanup",
			log.Cause(err))
	} else {
		log.Info(ctx, "Stripped monotonic suffix from channels.updated_at",
			log.Int64("affected", affected))
	}

	return nil
}
