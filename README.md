# PGXSchema

[![Go Reference](https://pkg.go.dev/badge/github.com/synaptor/pgxschema.svg)](https://pkg.go.dev/github.com/synaptor/pgxschema)
[![Build Status](https://github.com/synaptor/pgxschema/actions/workflows/test.yml/badge.svg)](https://github.com/synaptor/pgxschema/actions/workflows/test.yml)

`PGXSchema` is a Go library for managing Postgres database schemas with a focus on rollout safety and backwards/forwards data compatibility.

## Features

- Declarative schema definitions in Go.
- Automatic schema synchronization on application startup.
- No standalone schema migration tools needed.
- Support for progressive rollouts: multiple versions of the application won't fight for schema updates.
- Only safe schema changes are applied automatically (e.g., adding columns, creating tables, expanding types).
- Schema changes that are not safe (i.e., can cause data loss) are logged as warnings for manual execution.

## Usage

Call `pgxschema.Sync` right from your application server startup code:

```go
import (
    "github.com/synaptor/pgxschema"
)

func main() {
    // Initialize your server and database connection
    // ...

    // Update database schema
    if err := pgxschema.Sync(ctx, pool, &pgxschema.DatabaseSchema{
        Tables: []*pgxschema.TableSchema{
            {
                Name: "users",
                Columns: []*pgxschema.ColumnSchema{
                    {Name: "auth_provider", Type: pgxschema.ColumnTypeVarchar, Length: 50, Nullable: false},
                    {Name: "id", Type: pgxschema.ColumnTypeVarchar, Length: 50, Nullable: false},
                    {Name: "name", Type: pgxschema.ColumnTypeVarchar, Length: 100, Nullable: false},
                    {Name: "email", Type: pgxschema.ColumnTypeVarchar, Length: 255, Nullable: false},
                },

                // Multi-column primary key
                PrimaryKey: []string{"auth_provider", "id"},

                Indexes: []*pgxschema.IndexSchema{
                    // Single-column unique index
                    {Columns: []string{"email"}, Unique: true},
                },
            },
            {
                Name: "posts",
                Columns: []*pgxschema.ColumnSchema{
                    {Name: "id", Type: pgxschema.ColumnTypeSerial, Nullable: false},
                    {Name: "user_id", Type: pgxschema.ColumnTypeInteger, Nullable: false},
                    {Name: "title", Type: pgxschema.ColumnTypeVarchar, Length: 200, Nullable: false},
                    {Name: "content", Type: pgxschema.ColumnTypeText, Nullable: true},
                    {Name: "created_at", Type: pgxschema.ColumnTypeTimestamp, Nullable: false},
                },

                // Single-column primary key
                PrimaryKey: []string{"id"},

                Indexes: []*pgxschema.IndexSchema{
                    // Single-column non-unique index
                    {Columns: []string{"user_id"}},

                    // Multi-column non-unique index
                    {Columns: []string{"user_id", "created_at"}},
                },
            },
        },
    }); err != nil {
        log.Fatal(err)
    }

    // Start your server
    // ...
}
```

## More details on the philosophy of the library

Designing safe data migrations in distributed systems is a complex topic. For reliability reasons, changing the entire
application to a new version in lockstep is often not possible or desirable. Typically, operators want to perform a
progressive rollout, changing a small percentage of servers at a time. This means that multiple versions of the application
can be running simultaneously, and they must be compatible with the same database schema.

Certain changes are safe in this scenario. For example, adding a new column is safe because old versions of the application
will simply ignore it. The new version must be able to handle the case when the new column has its default value
(i.e., the old version of the application did not write any value to it), but at the same time it must not assume that all data readers
will know about the new column.

Removing a column, on the contrary, is not safe because old versions of the application might try to access the removed column,
which would lead to errors. Similarly, changing a column type in an incompatible way (e.g., changing an integer column to a string column)
is not safe because old versions of the application might not be able to parse the values anymore.

## Assumptions

- The library assumes that the schema is configured only by `pgxschema`. If the schema was changed manually, the behavior is undefined.

## Example of a safe migration process

Let's consider a practical example: changing a column `hostport` containing a host:port pair as a string to two separate columns
`host` and `port`:

- We add new columns `host` and `port` with default values of NULL. This is a safe operation. Nobody is using these columns yet.
- We deploy the new version of the application that writes to both `hostport` and the new `host` and `port` columns.
- Old versions of the application continue to read from and write to `hostport`.
- New versions of the application write to both `hostport` and the new `host` and `port` columns.
- New versions of the application read from `host` and `port`, and if they are NULL, they fall back to parsing `hostport`.
- Once old versions of the application are no longer running and rollbacks to the old version are no longer possible, we stop
  reading from `hostport`.
- We run a backfill operation to populate `host` and `port` for all existing rows based on the values in `hostport`.
- We wait for some more time to make sure we won't need to roll back to the version that accesses `host` and `port`.
- We finally drop the `hostport` column.

As you can see, only at the very last step, when there is no chance of rolling back to the old version of the application,
do we perform the unsafe operation of dropping a column.

`PGXSchema` only automates the safe steps of this process. It will automatically add new columns and tables, expand existing
columns, and perform other safe operations. All unsafe operations will be logged as warnings to prompt the operator to execute them manually when
they are certain that rollbacks to the old version of the application are not possible.

## Avoiding schema flapping

Although certain changes (like adding or dropping non-unique indexes) are generally safe in either direction, `PGXSchema` arbitrarily prefers
creating them automatically but dropping them manually. This avoids index flapping during progressive rollouts, when different
versions of the application come and go and would otherwise fight for control.

## Really? Manual migration steps in the 21st century?

It might be surprising, but yes. There is no way to reliably detect when it is safe to perform an unsafe operation automatically. Most ORM frameworks that facilitate schema migrations assume that the application is deployed in lockstep, and they do not support progressive
rollouts at all. A single `migrate.py` script cannot be aware of which versions of the application are currently running or whether it is safe to drop a column. Any such blind approach would inevitably lead to downtime.

If you feel brave, you can call `Plan` yourself and execute all the manual steps automatically. But be aware that this can lead to
downtime or data loss if you are not careful.

May your queries flow and the pager stay silent!

## License

MIT License
