package pgxschema

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type testDatabase struct {
	pool      *pgxpool.Pool
	container *postgres.PostgresContainer
}

func newTestDatabase(t *testing.T, version string, opts ...testcontainers.ContainerCustomizer) *testDatabase {
	ctx := t.Context()
	opts = append([]testcontainers.ContainerCustomizer{
		postgres.WithDatabase("test-db"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		postgres.BasicWaitStrategies(),
	}, opts...)

	pgContainer, err := postgres.Run(ctx, fmt.Sprintf("postgres:%s", version), opts...)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(pgContainer); err != nil {
			t.Error(err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable", "pool_max_conns=50")
	if err != nil {
		t.Fatal(err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return &testDatabase{pool: pool, container: pgContainer}
}
