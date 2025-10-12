package pgxschema

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Sync synchronizes the database schema to match the target schema.
func Sync(ctx context.Context, pool *pgxpool.Pool, target *DatabaseSchema) error {
	current, err := Read(ctx, pool)
	if err != nil {
		return err
	}
	automated, manual := Plan(current, target)
	for _, stmt := range automated {
		slog.InfoContext(ctx, "SQL migration", "statement", stmt)
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	for _, stmt := range manual {
		slog.WarnContext(ctx, "Manual migration pending", "statement", stmt)
	}
	return nil
}
