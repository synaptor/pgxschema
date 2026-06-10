package pgxschema

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/sync/semaphore"
)

func TestSync(t *testing.T) {
	// Ignore index names when comparing schemas, as they are auto-generated
	ignoreIndexNames := cmpopts.IgnoreFields(IndexSchema{}, "Name")

	// Limit parallelism to avoid overwhelming the system with too many containers.
	maxContainers := int64(10)
	if env := os.Getenv("PGXSCHEMA_MAX_CONTAINERS"); env != "" {
		if c, err := strconv.ParseInt(env, 10, 64); err != nil {
			t.Fatalf("invalid PGXSCHEMA_MAX_CONTAINERS: %v", err)
		} else {
			maxContainers = c
		}
	}
	sem := semaphore.NewWeighted(maxContainers)

	// Define which Postgres versions to test against. Test on something old and something new.
	pgVersions := []string{"13", "18"}
	if env := os.Getenv("PGXSCHEMA_PG_VERSIONS"); env != "" {
		pgVersions = strings.Split(env, ",")
	}

	for _, pgVersion := range pgVersions {
		t.Run(fmt.Sprintf("Postgres %s", pgVersion), func(t *testing.T) {
			for _, tt := range migrationTestCases {
				t.Run(tt.name, func(t *testing.T) {
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
					if diff := cmp.Diff(tt.current, schema, ignoreIndexNames); diff != "" {
						t.Fatalf("initial schema mismatch:\n%s", diff)
					}

					// Compute necessary migrations.
					automated, manual, err := Plan(tt.current, tt.target)
					if err != nil {
						t.Fatalf("Plan error: %v", err)
					}
					if diff := cmp.Diff(tt.wantAutomated, automated); diff != "" {
						t.Fatalf("proposed automated migrations:\n%s", diff)
					}
					if diff := cmp.Diff(tt.wantManual, manual); diff != "" {
						t.Fatalf("proposed manual migrations:\n%s", diff)
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
					if diff := cmp.Diff(tt.target, schema, ignoreIndexNames); diff != "" {
						t.Fatalf("final schema mismatch:\n%s", diff)
					}
				})
			}
		})
	}
}
