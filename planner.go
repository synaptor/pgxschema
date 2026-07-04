package pgxschema

import (
	"fmt"
	"reflect"
	"strings"
)

// Plan returns the SQL statements needed to migrate from the current schema to the target schema.
// First return value is the list of SQL statements to be executed automatically.
// Second return value is the list of SQL statements that need to be reviewed and executed manually.
func Plan(current, target *DatabaseSchema) (automated []string, manual []string, err error) {
	// Build maps for quick lookup
	currentTables := make(map[string]*TableSchema)
	for _, table := range current.Tables {
		currentTables[table.Name] = table
	}

	targetTables := make(map[string]*TableSchema)
	for _, table := range target.Tables {
		targetTables[table.Name] = table
	}

	// Process each target table
	for _, targetTable := range target.Tables {
		for _, col := range targetTable.Columns {
			if col.ArrayDims < 0 || col.ArrayDims > 1 {
				return nil, nil, fmt.Errorf("table %q, column %q: ArrayDims must be 0 or 1, got %d", targetTable.Name, col.Name, col.ArrayDims)
			}
			if col.ArrayDims == 1 && col.Type == ColumnTypeSerial {
				return nil, nil, fmt.Errorf("table %q, column %q: arrays of serial type are not supported", targetTable.Name, col.Name)
			}
		}

		currentTable := currentTables[targetTable.Name]

		if currentTable == nil {
			// Table doesn't exist, create it
			automated = append(automated, generateCreateTableStatements(targetTable)...)
		} else {
			// Check for forbidden columns before planning any changes
			forbidden := make(map[string]bool, len(targetTable.ForbiddenColumns))
			for _, col := range targetTable.ForbiddenColumns {
				forbidden[col] = true
			}
			for _, col := range currentTable.Columns {
				if forbidden[col.Name] {
					return nil, nil, fmt.Errorf("table %q contains forbidden column %q and can't be safely upgraded", targetTable.Name, col.Name)
				}
			}

			// Table exists, check for column changes
			auto, man, err := generateAlterTableSQL(currentTable, targetTable)
			if err != nil {
				return nil, nil, err
			}
			automated = append(automated, auto...)
			manual = append(manual, man...)
		}
	}

	// Find tables that need to be dropped
	for _, currentTable := range current.Tables {
		if targetTables[currentTable.Name] == nil {
			manual = append(manual, fmt.Sprintf("DROP TABLE %s", currentTable.Name))
		}
	}

	return automated, manual, nil
}

func generateCreateTableStatements(table *TableSchema) []string {
	var columns []string
	for _, col := range table.Columns {
		columns = append(columns, columnDefinition(col))
	}

	if len(table.PrimaryKey) > 0 {
		columns = append(columns, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(table.PrimaryKey, ", ")))
	}

	automated := []string{fmt.Sprintf("CREATE TABLE %s (%s)", table.Name, strings.Join(columns, ", "))}
	for _, idx := range table.Indexes {
		automated = append(automated, generateCreateIndexSQL(table.Name, idx))
	}
	return automated
}

// columnDefinition generates the column definition for a column
func columnDefinition(col *ColumnSchema) string {
	var parts []string
	parts = append(parts, col.Name)

	// Type
	var typeStr string
	switch col.Type {
	case ColumnTypeSerial:
		typeStr = "SERIAL"
	case ColumnTypeInteger:
		typeStr = "INTEGER"
	case ColumnTypeNumeric:
		switch {
		case col.Length > 0 && col.Precision > 0:
			typeStr = fmt.Sprintf("NUMERIC(%d, %d)", col.Length, col.Precision)
		case col.Length > 0:
			typeStr = fmt.Sprintf("NUMERIC(%d)", col.Length)
		default:
			typeStr = "NUMERIC"
		}
	case ColumnTypeVarchar:
		if col.Length > 0 {
			typeStr = fmt.Sprintf("VARCHAR(%d)", col.Length)
		} else {
			typeStr = "VARCHAR"
		}
	case ColumnTypeText:
		typeStr = "TEXT"
	case ColumnTypeBoolean:
		typeStr = "BOOLEAN"
	case ColumnTypeTimestamp:
		if col.WithTimezone {
			typeStr = "TIMESTAMP WITH TIME ZONE"
		} else {
			typeStr = "TIMESTAMP WITHOUT TIME ZONE"
		}
	case ColumnTypeBytes:
		typeStr = "BYTEA"
	default:
		typeStr = string(col.Type)
	}
	if col.ArrayDims == 1 {
		typeStr += "[]"
	}
	parts = append(parts, typeStr)

	// Nullable
	if !col.Nullable {
		parts = append(parts, "NOT NULL")
	}

	// Default
	if col.Default != "" {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", col.Default))
	}

	return strings.Join(parts, " ")
}

// generateAlterTableSQL generates ALTER TABLE statements for table changes
func generateAlterTableSQL(current, target *TableSchema) (automated []string, manual []string, err error) {
	// Build column maps
	currentColumns := make(map[string]*ColumnSchema)
	for _, col := range current.Columns {
		currentColumns[col.Name] = col
	}

	targetColumns := make(map[string]*ColumnSchema)
	for _, col := range target.Columns {
		targetColumns[col.Name] = col
	}

	// Collect all automated and manual actions
	var automatedActions []string
	var manualActions []string

	// Check for new or modified columns
	for _, targetCol := range target.Columns {
		currentCol := currentColumns[targetCol.Name]

		if currentCol == nil {
			// New column
			automatedActions = append(automatedActions, fmt.Sprintf("ADD COLUMN %s", columnDefinition(targetCol)))
		} else {
			// Check if column changed
			change := columnChanged(currentCol, targetCol)
			if change == changeNone {
				continue
			}
			if change == changeArray {
				return nil, nil, fmt.Errorf("table %q, column %q: cannot convert already-array column", target.Name, targetCol.Name)
			}

			actions := columnAlterActions(currentCol, targetCol)

			if change == changeSafe {
				automatedActions = append(automatedActions, actions...)
			} else {
				manualActions = append(manualActions, actions...)
			}
		}
	}

	// Check for dropped columns
	for _, currentCol := range current.Columns {
		if targetColumns[currentCol.Name] == nil {
			manualActions = append(manualActions, fmt.Sprintf("DROP COLUMN %s", currentCol.Name))
		}
	}

	// Check for primary key changes
	pkChange := primaryKeyChanged(current.PrimaryKey, target.PrimaryKey)
	if pkChange != changeNone {
		pkActions := generatePrimaryKeyActions(current.Name, current.PrimaryKey, target.PrimaryKey)
		if pkChange == changeSafe {
			automatedActions = append(automatedActions, pkActions...)
		} else {
			manualActions = append(manualActions, pkActions...)
		}
	}

	// Check for index changes
	idxAutoDrops, idxAutoCreates, idxManualDrops, idxManualCreates := generateIndexChanges(current, target)

	// Order matters:
	// 1. DROP INDEX (before column drops, since indexes may reference columns being dropped)
	// 2. ALTER TABLE (add/modify/drop columns)
	// 3. CREATE INDEX (after column adds, since indexes may reference new columns)

	automated = append(automated, idxAutoDrops...)
	manual = append(manual, idxManualDrops...)

	if len(automatedActions) > 0 {
		automated = append(automated, fmt.Sprintf("ALTER TABLE %s %s", target.Name, strings.Join(automatedActions, ", ")))
	}

	if len(manualActions) > 0 {
		manual = append(manual, fmt.Sprintf("ALTER TABLE %s %s", target.Name, strings.Join(manualActions, ", ")))
	}

	automated = append(automated, idxAutoCreates...)
	manual = append(manual, idxManualCreates...)

	return automated, manual, nil
}

// columnAlterActions generates the ALTER COLUMN actions for column changes
func columnAlterActions(current, target *ColumnSchema) []string {
	var actions []string

	// Type change
	if current.Type != target.Type || current.Length != target.Length || current.Precision != target.Precision || current.WithTimezone != target.WithTimezone || current.ArrayDims != target.ArrayDims {
		typeStr := ""
		switch target.Type {
		case ColumnTypeInteger:
			typeStr = "INTEGER"
		case ColumnTypeNumeric:
			if target.Length > 0 && target.Precision > 0 {
				typeStr = fmt.Sprintf("NUMERIC(%d, %d)", target.Length, target.Precision)
			} else if target.Length > 0 {
				typeStr = fmt.Sprintf("NUMERIC(%d)", target.Length)
			} else {
				typeStr = "NUMERIC"
			}
		case ColumnTypeVarchar:
			if target.Length > 0 {
				typeStr = fmt.Sprintf("VARCHAR(%d)", target.Length)
			} else {
				typeStr = "VARCHAR"
			}
		case ColumnTypeText:
			typeStr = "TEXT"
		case ColumnTypeBoolean:
			typeStr = "BOOLEAN"
		case ColumnTypeTimestamp:
			if target.WithTimezone {
				typeStr = "TIMESTAMP WITH TIME ZONE"
			} else {
				typeStr = "TIMESTAMP WITHOUT TIME ZONE"
			}
		case ColumnTypeBytes:
			typeStr = "BYTEA"
		default:
			typeStr = string(target.Type)
		}
		if target.ArrayDims == 1 {
			typeStr += "[]"
		}
		if current.ArrayDims == 0 && target.ArrayDims == 1 {
			actions = append(actions, fmt.Sprintf("ALTER COLUMN %s TYPE %s USING ARRAY[%s]", target.Name, typeStr, target.Name))
		} else {
			actions = append(actions, fmt.Sprintf("ALTER COLUMN %s TYPE %s", target.Name, typeStr))
		}
	}

	// Nullable change
	if current.Nullable != target.Nullable {
		if target.Nullable {
			actions = append(actions, fmt.Sprintf("ALTER COLUMN %s DROP NOT NULL", target.Name))
		} else {
			actions = append(actions, fmt.Sprintf("ALTER COLUMN %s SET NOT NULL", target.Name))
		}
	}

	// Default change
	if current.Default != target.Default {
		if target.Default != "" {
			actions = append(actions, fmt.Sprintf("ALTER COLUMN %s SET DEFAULT %s", target.Name, target.Default))
		} else {
			actions = append(actions, fmt.Sprintf("ALTER COLUMN %s DROP DEFAULT", target.Name))
		}
	}

	return actions
}

type change string

const (
	changeNone   change = "none"
	changeSafe   change = "safe"
	changeUnsafe change = "unsafe"
	changeArray  change = "array"
)

// columnChanged determines if a column has changed and whether the change is safe
func columnChanged(current, target *ColumnSchema) change {
	if current.Name != target.Name {
		return changeUnsafe
	}

	changed := false
	typeChanged := false

	// Scalar → array is unsafe but possible (caller emits USING ARRAY[col]).
	// Array → scalar has no meaningful automatic conversion.
	if current.ArrayDims != target.ArrayDims {
		if current.ArrayDims != 0 {
			return changeArray
		}
		return changeUnsafe
	}

	// Type changes - only allow safe expansions
	if current.Type != target.Type {
		typeChanged = true
		switch {
		case current.Type == ColumnTypeInteger && target.Type == ColumnTypeNumeric:
			changed = true
		case current.Type == ColumnTypeVarchar && target.Type == ColumnTypeText:
			changed = true
		default:
			return changeUnsafe
		}
	}

	// Length changes - increase is safe (only check if type didn't change)
	if !typeChanged && current.Length != target.Length {
		if target.Length < current.Length || target.Length == 0 || current.Length == 0 {
			return changeUnsafe
		}
		changed = true
	}

	// Precision changes (only check if type didn't change)
	if !typeChanged && current.Precision != target.Precision {
		if target.Precision < current.Precision {
			return changeUnsafe
		}
		changed = true
	}

	// Timezone changes
	if current.WithTimezone != target.WithTimezone {
		return changeUnsafe
	}

	// Nullable changes - NOT NULL -> NULL is safe
	if current.Nullable != target.Nullable {
		if current.Nullable {
			return changeUnsafe
		}
		changed = true
	}

	// Default changes - should be safe
	if current.Default != target.Default {
		changed = true
	}

	if changed {
		return changeSafe
	}
	return changeNone
}

// primaryKeyChanged determines if a primary key has changed and whether the change is safe
func primaryKeyChanged(current, target []string) change {
	if reflect.DeepEqual(current, target) {
		return changeNone
	}

	currentSet := make(map[string]bool)
	for _, col := range current {
		currentSet[col] = true
	}

	targetSet := make(map[string]bool)
	for _, col := range target {
		targetSet[col] = true
	}

	for _, col := range current {
		if !targetSet[col] {
			return changeUnsafe
		}
	}
	for _, col := range target {
		if !currentSet[col] {
			return changeUnsafe
		}
	}

	// It's just a reordering of columns, so it's safe
	return changeSafe
}

// generatePrimaryKeyActions generates the actions for primary key changes
func generatePrimaryKeyActions(tableName string, current, target []string) []string {
	var actions []string

	// If there's a current PK, we need to drop it first
	if len(current) > 0 {
		// We need to know the constraint name. PostgreSQL default is tablename_pkey
		actions = append(actions, fmt.Sprintf("DROP CONSTRAINT %s_pkey", tableName))
	}

	// If there's a target PK, add it
	if len(target) > 0 {
		actions = append(actions, fmt.Sprintf("ADD PRIMARY KEY (%s)", strings.Join(target, ", ")))
	}

	return actions
}

func generateIndexChanges(current, target *TableSchema) (autoDrops, autoCreates, manualDrops, manualCreates []string) {
	currentIndexes := make(map[string]*IndexSchema)
	for _, idx := range current.Indexes {
		currentIndexes[indexSignature(current.Name, idx)] = idx
	}

	targetIndexes := make(map[string]*IndexSchema)
	for _, idx := range target.Indexes {
		targetIndexes[indexSignature(target.Name, idx)] = idx
	}

	for sig, currentIdx := range currentIndexes {
		if targetIndexes[sig] == nil {
			manualDrops = append(manualDrops, fmt.Sprintf("DROP INDEX %s", currentIdx.Name))
		}
	}

	for sig, targetIdx := range targetIndexes {
		currentIdx := currentIndexes[sig]
		if currentIdx == nil {
			if targetIdx.Unique {
				manualCreates = append(manualCreates, generateCreateIndexSQL(target.Name, targetIdx))
			} else {
				autoCreates = append(autoCreates, generateCreateIndexSQL(target.Name, targetIdx))
			}
		}
	}

	return autoDrops, autoCreates, manualDrops, manualCreates
}

func normalizeIndexMethod(method IndexMethod) IndexMethod {
	if method == "" {
		return IndexMethodBtree
	}
	return method
}

func resolveIndexName(tableName string, idx *IndexSchema) string {
	if idx.Name != "" {
		return idx.Name
	}
	suffix := "idx"
	if idx.Unique {
		suffix = "key"
	}
	method := normalizeIndexMethod(idx.Method)
	if method == IndexMethodBtree {
		return fmt.Sprintf("%s_%s_%s", tableName, strings.Join(idx.Columns, "_"), suffix)
	}
	return fmt.Sprintf("%s_%s_%s_%s", tableName, strings.Join(idx.Columns, "_"), method, suffix)
}

func indexSignature(tableName string, idx *IndexSchema) string {
	return fmt.Sprintf("%s|%s|%v|%s", resolveIndexName(tableName, idx), normalizeIndexMethod(idx.Method), idx.Unique, strings.Join(idx.Columns, ","))
}

func generateCreateIndexSQL(tableName string, idx *IndexSchema) string {
	indexName := resolveIndexName(tableName, idx)
	method := normalizeIndexMethod(idx.Method)
	unique := ""
	if idx.Unique {
		unique = "UNIQUE "
	}
	return fmt.Sprintf("CREATE %sINDEX %s ON %s USING %s (%s)", unique, indexName, tableName, method, strings.Join(idx.Columns, ", "))
}
