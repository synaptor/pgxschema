package pgxschema

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/semaphore"
)

const maxSimultaneousContainers = 10

func TestSync(t *testing.T) {
	// Limit parallelism to avoid overwhelming the system with too many containers.
	sem := semaphore.NewWeighted(maxSimultaneousContainers)

	for _, pgVersion := range []string{"13", "14", "15", "16", "17", "18"} {
		for _, tt := range migrationTestCases {
			t.Run(fmt.Sprintf("Postgres %s: %s", pgVersion, tt.name), func(t *testing.T) {
				t.Parallel()
				ctx := t.Context()

				if err := sem.Acquire(ctx, 1); err != nil {
					t.Fatalf("acquiring semaphore: %v", err)
				}
				defer sem.Release(1)

				db := newTestDatabase(t, pgVersion)

				// Create initial schema.
				if err := Sync(ctx, db.pool, tt.current); err != nil {
					t.Fatalf("creating initial schema: %v", err)
				}

				// Ensure that the initial schema matches.
				schema, err := Read(ctx, db.pool)
				if err != nil {
					t.Fatalf("reading initial schema: %v", err)
				}
				if diff := cmp.Diff(tt.current, schema); diff != "" {
					t.Fatalf("initial schema mismatch:\n%s", diff)
				}

				// Apply migration.
				if err := Sync(ctx, db.pool, tt.target); err != nil {
					t.Fatalf("applying migration: %v", err)
				}

				// Automatically apply manual migrations, since we can't verify them otherwise.
				for _, stmt := range tt.wantManual {
					if _, err := db.pool.Exec(ctx, stmt); err != nil {
						t.Fatalf("applying manual migration %q: %v", stmt, err)
					}
				}

				// Ensure that the final schema matches.
				schema, err = Read(ctx, db.pool)
				if err != nil {
					t.Fatalf("reading final schema: %v", err)
				}
				if diff := cmp.Diff(tt.target, schema); diff != "" {
					t.Fatalf("final schema mismatch:\n%s", diff)
				}
			})
		}
	}
}
