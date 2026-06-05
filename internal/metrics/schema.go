package metrics

import (
	"fmt"
	"sort"

	iceberg "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
)

// SchemaChangeKind classifies one column-level change between two schemas.
type SchemaChangeKind string

const (
	ColumnAdded   SchemaChangeKind = "added"
	ColumnDropped SchemaChangeKind = "dropped"
	ColumnRenamed SchemaChangeKind = "renamed"
	ColumnRetyped SchemaChangeKind = "retyped"
)

// SchemaChange is a single column change detected by diffing two schemas.
// Diffs are keyed on the stable column ID, so a rename is a rename (same ID,
// new name) rather than a drop + add.
type SchemaChange struct {
	Kind     SchemaChangeKind `json:"kind"`
	ColumnID int              `json:"column_id"`
	Name     string           `json:"name"`
	OldName  string           `json:"old_name,omitempty"`
	NewName  string           `json:"new_name,omitempty"`
	OldType  string           `json:"old_type,omitempty"`
	NewType  string           `json:"new_type,omitempty"`
}

// SchemaVersionDiff is the set of changes between two consecutive schema versions.
type SchemaVersionDiff struct {
	FromSchemaID int            `json:"from_schema_id"`
	ToSchemaID   int            `json:"to_schema_id"`
	Changes      []SchemaChange `json:"changes"`
}

// SchemaReport summarizes a table's schema-evolution history.
type SchemaReport struct {
	CurrentSchemaID int                 `json:"current_schema_id"`
	TotalSchemas    int                 `json:"total_schemas"`
	CurrentColumns  int                 `json:"current_columns"`
	Diffs           []SchemaVersionDiff `json:"diffs"`
}

// AnalyzeSchema walks the full schema history and diffs each consecutive pair
// by stable column ID to produce a "what changed when" timeline.
func AnalyzeSchema(meta table.Metadata) SchemaReport {
	schemas := meta.Schemas() // []*iceberg.Schema

	// Defensive copy before sorting so we don't reorder the library's slice.
	ordered := append([]*iceberg.Schema(nil), schemas...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	report := SchemaReport{TotalSchemas: len(ordered)}
	if cur := meta.CurrentSchema(); cur != nil {
		report.CurrentSchemaID = cur.ID
		report.CurrentColumns = cur.NumFields()
	}

	for i := 1; i < len(ordered); i++ {
		diff := diffSchemas(ordered[i-1], ordered[i])
		if len(diff.Changes) > 0 {
			report.Diffs = append(report.Diffs, diff)
		}
	}
	return report
}

func diffSchemas(oldS, newS *iceberg.Schema) SchemaVersionDiff {
	oldFields := fieldsByID(oldS)
	newFields := fieldsByID(newS)

	var changes []SchemaChange

	// Walk the new schema: each column is new, renamed, retyped, or unchanged.
	for id, nf := range newFields {
		of, existed := oldFields[id]
		if !existed {
			changes = append(changes, SchemaChange{
				Kind: ColumnAdded, ColumnID: id, Name: nf.Name, NewType: fieldType(nf),
			})
			continue
		}
		if of.Name != nf.Name {
			changes = append(changes, SchemaChange{
				Kind: ColumnRenamed, ColumnID: id, Name: nf.Name, OldName: of.Name, NewName: nf.Name,
			})
		}
		if fieldType(of) != fieldType(nf) {
			changes = append(changes, SchemaChange{
				Kind: ColumnRetyped, ColumnID: id, Name: nf.Name, OldType: fieldType(of), NewType: fieldType(nf),
			})
		}
	}

	// Walk the old schema for drops: an ID present then but gone now.
	for id, of := range oldFields {
		if _, exists := newFields[id]; !exists {
			changes = append(changes, SchemaChange{
				Kind: ColumnDropped, ColumnID: id, Name: of.Name, OldType: fieldType(of),
			})
		}
	}

	// Stable, deterministic ordering for output and tests.
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].ColumnID != changes[j].ColumnID {
			return changes[i].ColumnID < changes[j].ColumnID
		}
		return changes[i].Kind < changes[j].Kind
	})

	return SchemaVersionDiff{FromSchemaID: oldS.ID, ToSchemaID: newS.ID, Changes: changes}
}

func fieldsByID(s *iceberg.Schema) map[int]iceberg.NestedField {
	m := make(map[int]iceberg.NestedField, s.NumFields())
	for _, f := range s.Fields() {
		m[f.ID] = f
	}
	return m
}

// fieldType renders an Iceberg field's type as a string (e.g. "long", "double").
func fieldType(f iceberg.NestedField) string {
	return fmt.Sprintf("%v", f.Type)
}
