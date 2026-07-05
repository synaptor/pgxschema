package pgxschema

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestSync(t *testing.T) {
	// Ignore index names when comparing schemas, as they are auto-generated.
	ignoreIndexNames := cmpopts.IgnoreFields(IndexSchema{}, "Name")
	compareIndexMethod := cmp.Comparer(func(a, b IndexMethod) bool {
		return normalizeIndexMethod(a) == normalizeIndexMethod(b)
	})
	compareIndexWhere := cmp.Comparer(func(a, b string) bool {
		return normalizeIndexWhere(a) == normalizeIndexWhere(b)
	})
	schemaCompareOpts := cmp.Options{
		ignoreIndexNames,
		compareIndexMethod,
		cmp.FilterPath(func(p cmp.Path) bool {
			return p.Last().String() == ".Where"
		}, compareIndexWhere),
	}

	// Define which Postgres versions to test against. Test on something old and something new.
	pgVersions := []string{"13", "14", "15", "16", "17", "18"}
	if env := os.Getenv("PGXSCHEMA_PG_VERSIONS"); env != "" {
		pgVersions = strings.Split(env, ",")
	}

	for _, pgVersion := range pgVersions {
		t.Run(fmt.Sprintf("pg%s", pgVersion), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			pgContainer, err := postgres.Run(ctx, fmt.Sprintf("postgres:%s-alpine", pgVersion),
				postgres.WithDatabase("test-db"),
				postgres.WithUsername("postgres"),
				postgres.WithPassword("postgres"),
				postgres.BasicWaitStrategies(),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := testcontainers.TerminateContainer(pgContainer); err != nil {
					t.Error(err)
				}
			})

			defaultConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
			if err != nil {
				t.Fatal(err)
			}

			for _, tt := range migrationTestCases {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					ctx := t.Context()

					dbName := testDatabaseName(t)

					adminConn, err := pgx.Connect(ctx, defaultConnStr)
					if err != nil {
						t.Fatalf("connecting to default database: %v", err)
					}
					if _, err := adminConn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
						adminConn.Close(ctx)
						t.Fatalf("creating database %q: %v", dbName, err)
					}
					if err := adminConn.Close(ctx); err != nil {
						t.Fatalf("closing admin connection: %v", err)
					}

					cfg, err := pgxpool.ParseConfig(defaultConnStr)
					if err != nil {
						t.Fatalf("parsing connection config: %v", err)
					}
					cfg.ConnConfig.Database = dbName
					cfg.MaxConns = 50

					pool, err := pgxpool.NewWithConfig(ctx, cfg)
					if err != nil {
						t.Fatalf("connecting to database %q: %v", dbName, err)
					}
					t.Cleanup(func() {
						pool.Close()
					})

					// Create initial schema.
					if err := Sync(ctx, pool, tt.current); err != nil {
						t.Fatalf("creating initial schema: %v", err)
					}

					// Ensure that the initial schema matches.
					schema, err := Read(ctx, pool)
					if err != nil {
						t.Fatalf("reading initial schema: %v", err)
					}
					if diff := cmp.Diff(tt.current, schema, schemaCompareOpts); diff != "" {
						t.Fatalf("initial schema mismatch:\n%s", diff)
					}

					// Ensure that diff is empty.
					automated, manual, err := Plan(schema, tt.current)
					if err != nil {
						t.Fatalf("planning unchanged schema: %v", err)
					}
					if len(automated) > 0 || len(manual) > 0 {
						t.Errorf("unchanged plan should be empty, but got %d automated and %d manual migrations", len(automated), len(manual))
					}

					// Compute necessary migrations.
					automated, manual, err = Plan(tt.current, tt.target)
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
					if err := Sync(ctx, pool, tt.target); err != nil {
						t.Fatalf("applying migration: %v", err)
					}

					// Automatically apply manual migrations, since we can't verify them otherwise.
					for _, stmt := range tt.wantManual {
						if _, err := pool.Exec(ctx, stmt); err != nil {
							t.Fatalf("applying manual migration %q: %v", stmt, err)
						}
					}

					// Ensure that the final schema matches.
					schema, err = Read(ctx, pool)
					if err != nil {
						t.Fatalf("reading final schema: %v", err)
					}
					if diff := cmp.Diff(tt.target, schema, schemaCompareOpts); diff != "" {
						t.Fatalf("final schema mismatch:\n%s", diff)
					}

					// Ensures that the final diff is empty.
					automated, manual, err = Plan(schema, tt.target)
					if err != nil {
						t.Fatalf("planning final schema: %v", err)
					}
					if len(automated) > 0 || len(manual) > 0 {
						t.Errorf("final plan should be empty, but got %d automated and %d manual migrations", len(automated), len(manual))
					}
				})
			}
		})
	}
}

func testDatabaseName(t *testing.T) string {
	var b strings.Builder
	for _, r := range strings.ToLower(t.Name()) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := b.String()
	if len(name) > 63 {
		t.Fatalf("database name %q is too long", name)
	}
	return name
}
