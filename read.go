package pgxschema

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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
				charMaxLength *int
				numericPrec   *int
				numericScale  *int
				isNullable    string
				columnDefault *string
			)

			if err := rows.Scan(&columnName, &dataType, &charMaxLength, &numericPrec, &numericScale, &isNullable, &columnDefault); err != nil {
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
		rows, err = pool.Query(ctx, sqlListPrimaryKey, "public."+tableName)
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
		schema.Tables = append(schema.Tables, table)
	}
	return schema, nil
}
