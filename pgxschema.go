// Package pgxschema is a opinionated database schema management library for Postgres.
//
// Documentation: https://github.com/synaptor/pgxschema
package pgxschema

import (
	"regexp"
)

const (
	sqlListTables = `
			SELECT table_name
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
			ORDER BY table_name
		`

	sqlListColumns = `
			SELECT 
				c.column_name,
				c.data_type,
				c.character_maximum_length,
				c.numeric_precision,
				c.numeric_scale,
				c.is_nullable,
				c.column_default
			FROM information_schema.columns c
			WHERE c.table_schema = 'public' 
				AND c.table_name = $1
			ORDER BY c.ordinal_position
		`

	sqlListPrimaryKey = `
			SELECT a.attname
			FROM pg_index i
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
			WHERE i.indrelid = $1::regclass
				AND i.indisprimary
			ORDER BY array_position(i.indkey, a.attnum)
		`

	sqlListIndexes = `
			SELECT 
				c.relname AS index_name,
				i.indisunique,
				a.attname
			FROM pg_index i
			JOIN pg_class c ON c.oid = i.indexrelid
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
			WHERE i.indrelid = $1::regclass
				AND NOT i.indisprimary
			ORDER BY c.relname, array_position(i.indkey, a.attnum)
		`
)

var (
	reDefaultValueType = regexp.MustCompile(`^(.+)::[a-z_ ]+$`)
)

// DatabaseSchema represents the schema of a database.
type DatabaseSchema struct {
	// Tables in the database.
	Tables []*TableSchema
}

// TableSchema represents the schema of a table.
type TableSchema struct {
	// Name of the table.
	Name string

	// Columns of the table.
	Columns []*ColumnSchema

	// Primary key columns (in order).
	PrimaryKey []string

	// Indexes on the table.
	Indexes []*IndexSchema
}

// IndexSchema represents the schema of an index.
type IndexSchema struct {
	// Name of the index (optional).
	Name string

	// Columns in the index (in order).
	Columns []string

	// Unique indicates if the index enforces uniqueness.
	Unique bool
}

// ColumnSchema represents the schema of a column.
type ColumnSchema struct {
	// Name of the column.
	Name string

	// Type of the column.
	Type ColumnType

	// Length of the column (for types that support it, e.g. varchar, decimal).
	Length int

	// Precision of the column (for types that support it, e.g. decimal).
	Precision int

	// WithTimezone indicates if the column has timezone information (for timestamp types).
	WithTimezone bool

	// Nullable indicates if the column is nullable.
	Nullable bool

	// Default value of the column (as an SQL expression).
	Default string
}

// ColumnType represents the base type of a column.
type ColumnType string

const (
	ColumnTypeInteger   ColumnType = "integer"
	ColumnTypeSerial    ColumnType = "serial"
	ColumnTypeNumeric   ColumnType = "numeric"
	ColumnTypeVarchar   ColumnType = "varchar"
	ColumnTypeText      ColumnType = "text"
	ColumnTypeBoolean   ColumnType = "boolean"
	ColumnTypeTimestamp ColumnType = "timestamp"
	ColumnTypeBytes     ColumnType = "bytes"
)
