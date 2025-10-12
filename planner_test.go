package pgxschema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	migrationTestCases = []struct {
		name          string
		current       *DatabaseSchema
		target        *DatabaseSchema
		wantAutomated []string
		wantManual    []string
	}{
		{
			name:    "empty to empty",
			current: &DatabaseSchema{},
			target:  &DatabaseSchema{},
		},
		{
			name:    "create new table",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE users (id SERIAL NOT NULL, name VARCHAR(100) NOT NULL, PRIMARY KEY (id))"},
		},
		{
			name: "multiple alter actions merged",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: true},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: true},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN name TYPE VARCHAR(100), ADD COLUMN email VARCHAR(255) NOT NULL"},
		},
		{
			name: "manual and automated actions separated",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
							{Name: "old_field", Type: ColumnTypeText, Nullable: true},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Length: 200, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN name TYPE VARCHAR(200)"},
			wantManual:    []string{"ALTER TABLE users DROP COLUMN old_field"},
		},
		{
			name: "drop table",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "old_table",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target:     &DatabaseSchema{},
			wantManual: []string{"DROP TABLE old_table"},
		},
		{
			name:    "create multiple tables",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "posts",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			wantAutomated: []string{
				"CREATE TABLE posts (id SERIAL NOT NULL, PRIMARY KEY (id))",
				"CREATE TABLE users (id SERIAL NOT NULL, PRIMARY KEY (id))",
			},
		},
		{
			name: "safe type expansion - varchar to text",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "articles",
						Columns: []*ColumnSchema{
							{Name: "content", Type: ColumnTypeVarchar, Length: 1000, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "articles",
						Columns: []*ColumnSchema{
							{Name: "content", Type: ColumnTypeText, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE articles ALTER COLUMN content TYPE TEXT"},
		},
		{
			name: "safe type expansion - integer to numeric",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "prices",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeInteger, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "prices",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Length: 10, Precision: 2, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE prices ALTER COLUMN amount TYPE NUMERIC(10, 2)"},
		},
		{
			name: "unsafe type change - text to varchar",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "articles",
						Columns: []*ColumnSchema{
							{Name: "content", Type: ColumnTypeText, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "articles",
						Columns: []*ColumnSchema{
							{Name: "content", Type: ColumnTypeVarchar, Length: 1000, Nullable: false},
						},
					},
				},
			},
			wantManual: []string{"ALTER TABLE articles ALTER COLUMN content TYPE VARCHAR(1000)"},
		},
		{
			name: "safe length increase",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "email", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN email TYPE VARCHAR(255)"},
		},
		{
			name: "unsafe length decrease",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "code", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "code", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
					},
				},
			},
			wantManual: []string{"ALTER TABLE users ALTER COLUMN code TYPE VARCHAR(50)"},
		},
		{
			name: "safe nullable change - NOT NULL to NULL",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "phone", Type: ColumnTypeVarchar, Length: 20, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "phone", Type: ColumnTypeVarchar, Length: 20, Nullable: true},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN phone DROP NOT NULL"},
		},
		{
			name: "unsafe nullable change - NULL to NOT NULL",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "phone", Type: ColumnTypeVarchar, Length: 20, Nullable: true},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "phone", Type: ColumnTypeVarchar, Length: 20, Nullable: false},
						},
					},
				},
			},
			wantManual: []string{"ALTER TABLE users ALTER COLUMN phone SET NOT NULL"},
		},
		{
			name: "add default value",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: false, Default: "'active'"},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN status SET DEFAULT 'active'"},
		},
		{
			name: "remove default value",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: false, Default: "'active'"},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN status DROP DEFAULT"},
		},
		{
			name: "change default value",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: false, Default: "'active'"},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "status", Type: ColumnTypeVarchar, Length: 20, Nullable: false, Default: "'pending'"},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN status SET DEFAULT 'pending'"},
		},
		{
			name: "multiple column changes with type, nullable, and default",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "bio", Type: ColumnTypeVarchar, Length: 500, Nullable: false, Default: "''"},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "bio", Type: ColumnTypeText, Nullable: true},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE users ALTER COLUMN bio TYPE TEXT, ALTER COLUMN bio DROP NOT NULL, ALTER COLUMN bio DROP DEFAULT"},
		},
		{
			name:    "all column types",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "all_types",
						Columns: []*ColumnSchema{
							{Name: "col_int", Type: ColumnTypeInteger, Nullable: false},
							{Name: "col_serial", Type: ColumnTypeSerial, Nullable: false},
							{Name: "col_numeric", Type: ColumnTypeNumeric, Length: 10, Precision: 2, Nullable: false},
							{Name: "col_varchar", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
							{Name: "col_text", Type: ColumnTypeText, Nullable: false},
							{Name: "col_bool", Type: ColumnTypeBoolean, Nullable: false},
							{Name: "col_timestamp", Type: ColumnTypeTimestamp, Nullable: false},
							{Name: "col_timestamp_tz", Type: ColumnTypeTimestamp, WithTimezone: true, Nullable: false},
							{Name: "col_bytes", Type: ColumnTypeBytes, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{
				"CREATE TABLE all_types (col_int INTEGER NOT NULL, col_serial SERIAL NOT NULL, col_numeric NUMERIC(10, 2) NOT NULL, col_varchar VARCHAR(100) NOT NULL, col_text TEXT NOT NULL, col_bool BOOLEAN NOT NULL, col_timestamp TIMESTAMP WITHOUT TIME ZONE NOT NULL, col_timestamp_tz TIMESTAMP WITH TIME ZONE NOT NULL, col_bytes BYTEA NOT NULL)",
			},
		},
		{
			name:    "composite primary key",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "user_roles",
						Columns: []*ColumnSchema{
							{Name: "user_id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "role_id", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"user_id", "role_id"},
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE user_roles (user_id INTEGER NOT NULL, role_id INTEGER NOT NULL, PRIMARY KEY (user_id, role_id))"},
		},
		{
			name: "no changes needed",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
		},
		{
			name: "precision increase is safe",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "prices",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Length: 10, Precision: 2, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "prices",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Length: 10, Precision: 4, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE prices ALTER COLUMN amount TYPE NUMERIC(10, 4)"},
		},
		{
			name: "precision decrease is unsafe",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "prices",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Length: 10, Precision: 4, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "prices",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Length: 10, Precision: 2, Nullable: false},
						},
					},
				},
			},
			wantManual: []string{"ALTER TABLE prices ALTER COLUMN amount TYPE NUMERIC(10, 2)"},
		},
		{
			name: "timezone change is unsafe",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "events",
						Columns: []*ColumnSchema{
							{Name: "created_at", Type: ColumnTypeTimestamp, WithTimezone: false, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "events",
						Columns: []*ColumnSchema{
							{Name: "created_at", Type: ColumnTypeTimestamp, WithTimezone: true, Nullable: false},
						},
					},
				},
			},
			wantManual: []string{"ALTER TABLE events ALTER COLUMN created_at TYPE TIMESTAMP WITH TIME ZONE"},
		},
		{
			name: "mixed safe and unsafe changes across multiple tables",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "posts",
						Columns: []*ColumnSchema{
							{Name: "title", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
					},
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
							{Name: "old_col", Type: ColumnTypeText, Nullable: true},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "posts",
						Columns: []*ColumnSchema{
							{Name: "title", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
					},
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{
				"ALTER TABLE users ALTER COLUMN name TYPE VARCHAR(100), ADD COLUMN email VARCHAR(255) NOT NULL",
			},
			wantManual: []string{
				"ALTER TABLE posts ALTER COLUMN title TYPE VARCHAR(50)",
				"ALTER TABLE users DROP COLUMN old_col",
			},
		},
		{
			name: "unsafe primary key change",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "items",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "items",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
						},
						// PrimaryKey defaults to nil (removing primary key)
					},
				},
			},
			wantManual: []string{"ALTER TABLE items DROP CONSTRAINT items_pkey"},
		},
		{
			name: "add column with all attributes",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "products",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "products",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "price", Type: ColumnTypeNumeric, Length: 10, Precision: 2, Nullable: false, Default: "0"},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE products ADD COLUMN price NUMERIC(10, 2) NOT NULL DEFAULT 0"},
		},
		{
			name:    "numeric without precision",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "values",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Length: 10, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE values (amount NUMERIC(10) NOT NULL)"},
		},
		{
			name:    "numeric without length or precision",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "values",
						Columns: []*ColumnSchema{
							{Name: "amount", Type: ColumnTypeNumeric, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE values (amount NUMERIC NOT NULL)"},
		},
		{
			name:    "varchar without length",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "items",
						Columns: []*ColumnSchema{
							{Name: "name", Type: ColumnTypeVarchar, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE items (name VARCHAR NOT NULL)"},
		},
		{
			name:    "empty table without columns",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "empty",
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE empty ()"},
		},
		{
			name: "multiple tables with mixed operations",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "keep_unchanged",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
						},
					},
					{
						Name: "to_be_altered",
						Columns: []*ColumnSchema{
							{Name: "col1", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
					},
					{
						Name: "to_be_dropped",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "keep_unchanged",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
						},
					},
					{
						Name: "newly_created",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
					{
						Name: "to_be_altered",
						Columns: []*ColumnSchema{
							{Name: "col1", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{
				"CREATE TABLE newly_created (id SERIAL NOT NULL, PRIMARY KEY (id))",
				"ALTER TABLE to_be_altered ALTER COLUMN col1 TYPE VARCHAR(100)",
			},
			wantManual: []string{"DROP TABLE to_be_dropped"},
		},
		{
			name:    "column with nullable and default",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "configs",
						Columns: []*ColumnSchema{
							{Name: "enabled", Type: ColumnTypeBoolean, Nullable: true, Default: "false"},
						},
					},
				},
			},
			wantAutomated: []string{"CREATE TABLE configs (enabled BOOLEAN DEFAULT false)"},
		},
		{
			name: "only default change safe",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "settings",
						Columns: []*ColumnSchema{
							{Name: "timeout", Type: ColumnTypeInteger, Nullable: false, Default: "30"},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "settings",
						Columns: []*ColumnSchema{
							{Name: "timeout", Type: ColumnTypeInteger, Nullable: false, Default: "60"},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE settings ALTER COLUMN timeout SET DEFAULT 60"},
		},
		{
			name: "length increase with same type",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "data",
						Columns: []*ColumnSchema{
							{Name: "code", Type: ColumnTypeVarchar, Length: 10, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "data",
						Columns: []*ColumnSchema{
							{Name: "code", Type: ColumnTypeVarchar, Length: 20, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE data ALTER COLUMN code TYPE VARCHAR(20)"},
		},
		{
			name: "numeric length increase with same precision",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "finances",
						Columns: []*ColumnSchema{
							{Name: "balance", Type: ColumnTypeNumeric, Length: 10, Precision: 2, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "finances",
						Columns: []*ColumnSchema{
							{Name: "balance", Type: ColumnTypeNumeric, Length: 15, Precision: 2, Nullable: false},
						},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE finances ALTER COLUMN balance TYPE NUMERIC(15, 2)"},
		},
		{
			name: "reorder primary key columns - safe",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "user_roles",
						Columns: []*ColumnSchema{
							{Name: "user_id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "role_id", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"user_id", "role_id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "user_roles",
						Columns: []*ColumnSchema{
							{Name: "user_id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "role_id", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"role_id", "user_id"}, // Different order
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE user_roles DROP CONSTRAINT user_roles_pkey, ADD PRIMARY KEY (role_id, user_id)"},
		},
		{
			name: "add column to primary key",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "events",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "timestamp", Type: ColumnTypeTimestamp, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "events",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "timestamp", Type: ColumnTypeTimestamp, Nullable: false},
						},
						PrimaryKey: []string{"id", "timestamp"},
					},
				},
			},
			wantManual: []string{"ALTER TABLE events DROP CONSTRAINT events_pkey, ADD PRIMARY KEY (id, timestamp)"},
		},
		{
			name: "remove column from primary key",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "orders",
						Columns: []*ColumnSchema{
							{Name: "order_id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "line_number", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"order_id", "line_number"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "orders",
						Columns: []*ColumnSchema{
							{Name: "order_id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "line_number", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"order_id"},
					},
				},
			},
			wantManual: []string{"ALTER TABLE orders DROP CONSTRAINT orders_pkey, ADD PRIMARY KEY (order_id)"},
		},
		{
			name: "change primary key to different columns",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "products",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "sku", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "products",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "sku", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
						PrimaryKey: []string{"sku"},
					},
				},
			},
			wantManual: []string{"ALTER TABLE products DROP CONSTRAINT products_pkey, ADD PRIMARY KEY (sku)"},
		},
		{
			name: "add primary key to table without one",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "logs",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "message", Type: ColumnTypeText, Nullable: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "logs",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeInteger, Nullable: false},
							{Name: "message", Type: ColumnTypeText, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			wantManual: []string{"ALTER TABLE logs ADD PRIMARY KEY (id)"},
		},
		{
			name: "multi-column primary key with column order change and addition",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "composite_pk",
						Columns: []*ColumnSchema{
							{Name: "col_a", Type: ColumnTypeInteger, Nullable: false},
							{Name: "col_b", Type: ColumnTypeInteger, Nullable: false},
							{Name: "col_c", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"col_a", "col_b"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "composite_pk",
						Columns: []*ColumnSchema{
							{Name: "col_a", Type: ColumnTypeInteger, Nullable: false},
							{Name: "col_b", Type: ColumnTypeInteger, Nullable: false},
							{Name: "col_c", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"col_b", "col_a", "col_c"},
					},
				},
			},
			wantManual: []string{"ALTER TABLE composite_pk DROP CONSTRAINT composite_pk_pkey, ADD PRIMARY KEY (col_b, col_a, col_c)"},
		},
		{
			name: "simple reorder of primary key columns",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "reorder_pk",
						Columns: []*ColumnSchema{
							{Name: "first", Type: ColumnTypeInteger, Nullable: false},
							{Name: "second", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"first", "second"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "reorder_pk",
						Columns: []*ColumnSchema{
							{Name: "first", Type: ColumnTypeInteger, Nullable: false},
							{Name: "second", Type: ColumnTypeInteger, Nullable: false},
						},
						PrimaryKey: []string{"second", "first"},
					},
				},
			},
			wantAutomated: []string{"ALTER TABLE reorder_pk DROP CONSTRAINT reorder_pk_pkey, ADD PRIMARY KEY (second, first)"},
		},
		{
			name:    "create table with index",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"email"}},
						},
					},
				},
			},
			wantAutomated: []string{
				"CREATE TABLE users (id SERIAL NOT NULL, email VARCHAR(255) NOT NULL, PRIMARY KEY (id))",
				"CREATE INDEX users_email_idx ON users (email)",
			},
		},
		{
			name:    "create table with unique index",
			current: &DatabaseSchema{},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"email"}, Unique: true},
						},
					},
				},
			},
			wantAutomated: []string{
				"CREATE TABLE users (id SERIAL NOT NULL, email VARCHAR(255) NOT NULL, PRIMARY KEY (id))",
				"CREATE UNIQUE INDEX users_email_key ON users (email)",
			},
		},
		{
			name: "add index to existing table",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"email"}},
						},
					},
				},
			},
			wantAutomated: []string{"CREATE INDEX users_email_idx ON users (email)"},
		},
		{
			name: "add unique index to existing table",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"email"}, Unique: true},
						},
					},
				},
			},
			wantManual: []string{"CREATE UNIQUE INDEX users_email_key ON users (email)"},
		},
		{
			name: "drop index from table",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Name: "users_email_idx", Columns: []string{"email"}},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			wantManual: []string{"DROP INDEX users_email_idx"},
		},
		{
			name: "change index columns",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "first_name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
							{Name: "last_name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Name: "users_first_name_idx", Columns: []string{"first_name"}},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "first_name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
							{Name: "last_name", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"first_name", "last_name"}},
						},
					},
				},
			},
			wantAutomated: []string{
				"CREATE INDEX users_first_name_last_name_idx ON users (first_name, last_name)",
			},
			wantManual: []string{
				"DROP INDEX users_first_name_idx",
			},
		},
		{
			name: "change index from non-unique to unique",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Name: "users_email_idx", Columns: []string{"email"}, Unique: false},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"email"}, Unique: true},
						},
					},
				},
			},
			wantManual: []string{
				"DROP INDEX users_email_idx",
				"CREATE UNIQUE INDEX users_email_key ON users (email)",
			},
		},
		{
			name: "multiple indexes",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
							{Name: "username", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
						PrimaryKey: []string{"id"},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "id", Type: ColumnTypeSerial, Nullable: false},
							{Name: "email", Type: ColumnTypeVarchar, Length: 255, Nullable: false},
							{Name: "username", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
						},
						PrimaryKey: []string{"id"},
						Indexes: []*IndexSchema{
							{Columns: []string{"email"}},
							{Columns: []string{"username"}, Unique: true},
						},
					},
				},
			},
			wantAutomated: []string{
				"CREATE INDEX users_email_idx ON users (email)",
			},
			wantManual: []string{
				"CREATE UNIQUE INDEX users_username_key ON users (username)",
			},
		},
		{
			name: "no change in the index",
			current: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "backend_type", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
							{Name: "backend_id", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						Indexes: []*IndexSchema{
							{Name: "some_existing_name", Columns: []string{"backend_type", "backend_id"}},
						},
					},
				},
			},
			target: &DatabaseSchema{
				Tables: []*TableSchema{
					{
						Name: "users",
						Columns: []*ColumnSchema{
							{Name: "backend_type", Type: ColumnTypeVarchar, Length: 50, Nullable: false},
							{Name: "backend_id", Type: ColumnTypeVarchar, Length: 100, Nullable: false},
						},
						Indexes: []*IndexSchema{
							{Columns: []string{"backend_type", "backend_id"}},
						},
					},
				},
			},
		},
	}
)

func TestPlanner(t *testing.T) {
	for _, tt := range migrationTestCases {
		t.Run(tt.name, func(t *testing.T) {
			automated, manual := Plan(tt.current, tt.target)
			if len(tt.wantAutomated) == 0 {
				tt.wantAutomated = []string(nil)
			}
			if len(tt.wantManual) == 0 {
				tt.wantManual = []string(nil)
			}
			if diff := cmp.Diff(tt.wantAutomated, automated); diff != "" {
				t.Errorf("automated:\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantManual, manual); diff != "" {
				t.Errorf("manual:\n%s", diff)
			}
		})
	}
}
