package pgxschema

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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
				c.udt_name,
				c.character_maximum_length,
				c.numeric_precision,
				c.numeric_scale,
				c.is_nullable,
				c.column_default,
				a.atttypmod
			FROM information_schema.columns c
			JOIN pg_catalog.pg_attribute a
				ON  a.attrelid = (c.table_schema || '.' || c.table_name)::regclass
				AND a.attname  = c.column_name
				AND NOT a.attisdropped
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
				am.amname,
				a.attname
			FROM pg_index i
			JOIN pg_class c ON c.oid = i.indexrelid
			JOIN pg_am am ON am.oid = c.relam
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
			WHERE i.indrelid = $1::regclass
				AND NOT i.indisprimary
			ORDER BY c.relname, array_position(i.indkey, a.attnum)
		`
)

var (
	reDefaultValueType = regexp.MustCompile(`^(.+)::[a-z_ ]+$`)
)

// Read reads the existing database schema.
func Read(ctx context.Context, pool *pgxpool.Pool) (*DatabaseSchema, error) {
	rows, err := pool.Query(ctx, sqlListTables)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tableNames = append(tableNames, tableName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading table names: %w", err)
	}

	schema := new(DatabaseSchema)
	for _, tableName := range tableNames {
		table := &TableSchema{Name: tableName}
		rows, err := pool.Query(ctx, sqlListColumns, tableName)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				columnName    string
				dataType      string
				udtName       string
				charMaxLength *int
				numericPrec   *int
				numericScale  *int
				isNullable    string
				columnDefault *string
				atttypmod     int
			)

			if err := rows.Scan(&columnName, &dataType, &udtName, &charMaxLength, &numericPrec, &numericScale, &isNullable, &columnDefault, &atttypmod); err != nil {
				return nil, fmt.Errorf("scanning column for table %s: %w", tableName, err)
			}

			col := &ColumnSchema{
				Name:     columnName,
				Nullable: isNullable == "YES",
			}

			// Map PostgreSQL types to our column types
			switch dataType {
			case "integer":
				col.Type = ColumnTypeInteger
				if columnDefault != nil && strings.HasPrefix(*columnDefault, "nextval(") {
					col.Type = ColumnTypeSerial
					columnDefault = nil
				}
			case "character varying", "varchar":
				col.Type = ColumnTypeVarchar
				if charMaxLength != nil {
					col.Length = *charMaxLength
				}
			case "text":
				col.Type = ColumnTypeText
			case "boolean":
				col.Type = ColumnTypeBoolean
			case "timestamp without time zone":
				col.Type = ColumnTypeTimestamp
			case "timestamp with time zone":
				col.Type = ColumnTypeTimestamp
				col.WithTimezone = true
			case "bytea":
				col.Type = ColumnTypeBytes
			case "numeric":
				col.Type = ColumnTypeNumeric
				if numericScale != nil {
					col.Precision = *numericScale
				}
				if numericPrec != nil {
					col.Length = *numericPrec
				}
			case "ARRAY":
				col.ArrayDims = 1
				switch udtName {
				case "_int4":
					col.Type = ColumnTypeInteger
				case "_numeric":
					col.Type = ColumnTypeNumeric
					// atttypmod stores element precision and scale: encoded = atttypmod - 4,
					// precision = encoded >> 16, scale = encoded & 0xFFFF.
					if atttypmod > 0 {
						encoded := atttypmod - 4
						col.Length = (encoded >> 16) & 0xFFFF
						col.Precision = encoded & 0xFFFF
					}
				case "_varchar":
					col.Type = ColumnTypeVarchar
					// atttypmod stores element length: length = atttypmod - 4.
					if atttypmod > 0 {
						col.Length = atttypmod - 4
					}
				case "_text":
					col.Type = ColumnTypeText
				case "_bool":
					col.Type = ColumnTypeBoolean
				case "_timestamp":
					col.Type = ColumnTypeTimestamp
				case "_timestamptz":
					col.Type = ColumnTypeTimestamp
					col.WithTimezone = true
				case "_bytea":
					col.Type = ColumnTypeBytes
				default:
					return nil, fmt.Errorf("unsupported array element type %q for column %q in table %q", udtName, columnName, tableName)
				}
			default:
				return nil, fmt.Errorf("unsupported data type %q for column %q in table %q", dataType, columnName, tableName)
			}
			if columnDefault != nil {
				col.Default = *columnDefault
				if m := reDefaultValueType.FindStringSubmatch(col.Default); m != nil {
					col.Default = m[1]
				}
			}
			table.Columns = append(table.Columns, col)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("reading columns for table %s: %w", tableName, err)
		}

		// Read primary key constraint
		rows, err = pool.Query(ctx, sqlListPrimaryKey, fmt.Sprintf("public.%s", tableName))
		if err != nil {
			return nil, fmt.Errorf("reading primary key for table %s: %w", tableName, err)
		}
		defer rows.Close()
		for rows.Next() {
			var columnName string
			if err := rows.Scan(&columnName); err != nil {
				return nil, fmt.Errorf("scanning primary key column for table %s: %w", tableName, err)
			}
			table.PrimaryKey = append(table.PrimaryKey, columnName)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("reading primary key for table %s: %w", tableName, err)
		}

		rows, err = pool.Query(ctx, sqlListIndexes, fmt.Sprintf("public.%s", tableName))
		if err != nil {
			return nil, fmt.Errorf("reading indexes for table %s: %w", tableName, err)
		}
		defer rows.Close()

		indexMap := make(map[string]*IndexSchema)
		for rows.Next() {
			var indexName string
			var isUnique bool
			var method string
			var columnName string
			if err := rows.Scan(&indexName, &isUnique, &method, &columnName); err != nil {
				return nil, fmt.Errorf("scanning index for table %s: %w", tableName, err)
			}

			if indexMap[indexName] == nil {
				indexMap[indexName] = &IndexSchema{
					Name:   indexName,
					Unique: isUnique,
					Method: IndexMethod(method),
				}
			}
			indexMap[indexName].Columns = append(indexMap[indexName].Columns, columnName)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("reading indexes for table %s: %w", tableName, err)
		}

		// Deterministically sort indexes by name
		indexNames := make([]string, 0, len(indexMap))
		for name := range indexMap {
			indexNames = append(indexNames, name)
		}
		sort.Strings(indexNames)

		for _, name := range indexNames {
			table.Indexes = append(table.Indexes, indexMap[name])
		}

		schema.Tables = append(schema.Tables, table)
	}
	return schema, nil
}
