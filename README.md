# PGX Schema Migration Library

A Go library for managing Postgres database schemas with a focus on rollout safety and backwards/forwards data compatibility.

## Features

- Declarative schema definitions in Go.
- Automatic schema synchronization on the application startup.
- No standalone schema migration tools needed.
- Support for progressive rollouts: multiple versions of the application won't fight for schema updates.
- Only safe schema changes are applied automatically (e.g., adding columns, creating tables, expanding types).
- Schema changes that are not safe (i.e. can cause data loss) are logged as warnings for manual execution.

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
                    {Name: "id", Type: pgxschema.ColumnTypeSerial, PrimaryKey: true},
                    {Name: "name", Type: pgxschema.ColumnTypeVarchar, Length: 100},
                    {Name: "email", Type: pgxschema.ColumnTypeVarchar, Length: 100},
                },
            },
            {
                Name: "posts",
                Columns: []*pgxschema.ColumnSchema{
                    {Name: "id", Type: pgxschema.ColumnTypeSerial, PrimaryKey: true},
                    {Name: "user_id", Type: pgxschema.ColumnTypeInt},
                    {Name: "content", Type: pgxschema.ColumnTypeText},
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

Designing safe data migrations in distributed systems is a complex topic. For the reliability reasons, changing the whole
application to a new version in a lockstep is often not possible or desirable. Typically, operators want to do a
progressive rollout, changing a small percentage of servers at a time. This means that multiple versions of the application
can be running at the same time, and they must be compatible with the same database schema.

Certain changes are safe to do in this scenario. For example, adding a new column is safe, because old versions of the application
will simply ignore the new column. The new version must be able to handle the case when the new column has its default value
(i.e. the old version of the application did not write any value to it), but at the same time it must not assume that all data readers
will know about the new column.

Removing a column, on the contrary, is not safe, because old versions of the application might try to access the removed column,
which would lead to errors. Similarly, changing a column type in an incompatible way (e.g. changing an integer column to a string column)
is not safe, because old versions of the application might not be able to parse values anymore.

## Example of a safe migration process

Let's consider a practical example: changing a column `hostport` containing a host:port pair as a string to two separate columns
`host` and `port`:

- We add new columns `host` and `port` with default values of NULL. This is a safe operation. Nobody is using these columns yet.
- We deploy the new version of the application that writes to both `hostport` and the new `host` and `port` columns.
- Old versions of the application continue to read and write to `hostport`
- New versions of the application write to both `hostport` and the new `host` and `port` columns.
- New versions of the application read from `host` and `port`, and if they are NULL, they fall back to parsing `hostport`.
- Once old versions of the application are no longer running, and rollbacks to the old version are not possible anymore, we stop
  reading `host` and `port`.
- Again, we wait rollbacks to the version that accesses `host` and `port` are not possible anymore, and then we finally drop the
  `hostport` column.

As you can see, only at the very last step, when there is no single chance of rolling back to the old version of the application,
we do an unsafe operation of dropping a column.

PGXSchema only automates the safe steps of this process. It will automatically add new columns and tables, add more space to existing
columns, and do other safe operations. All unsafe operations will be logged as warnings to prompt the operator to do them manually when
they are sure that rollbacks to the old version of the application are not possible.

## Avoiding schema flapping

Although certain changes (like adding or dropping non-unique indexes) are generally safe in either direction, PGXSchema arbitrarily prefers
creating them automatically, but dropping them manually. This is to avoid index flapping during progressive rollouts, when different
versions of the application come and go and would fight otherwise.

## Really? Manual migration steps in 21st century?

Yes. There is no way to reliably detect when it is safe to do an unsafe operation automatically. Most of the ORM frameworks assume
that the application is deployed in a lockstep, and they do not support progressive rollouts at all. A single `migrate.py` script
can't be aware of which versions of the application are currently running, and whether it is safe to drop a column or not. Any such
blind approach would inevitably lead to downtime.

If you feel brave, you can call `Plan` yourself, and execute all the manual steps automatically. But be aware that this can lead to
downtime or data loss if you are not careful.

May your queries flow and the pager stay silent!

## License

MIT License
