package pgxschema

import (
	"context"

	"github.com/golang/glog"
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
		glog.Infof("SQL migration: %s", stmt)
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	for _, stmt := range manual {
		glog.Warningf("Manual migration pending: %s", stmt)
	}
	return nil
}
