package metrics

import "testing"

func TestAnalyzeSchema(t *testing.T) {
	meta := loadTestMeta(t)
	got := AnalyzeSchema(meta)

	if got.TotalSchemas != 2 {
		t.Errorf("TotalSchemas = %d, want 2", got.TotalSchemas)
	}
	if got.CurrentSchemaID != 1 {
		t.Errorf("CurrentSchemaID = %d, want 1", got.CurrentSchemaID)
	}
	if got.CurrentColumns != 3 {
		t.Errorf("CurrentColumns = %d, want 3", got.CurrentColumns)
	}
	if len(got.Diffs) != 1 {
		t.Fatalf("len(Diffs) = %d, want 1", len(got.Diffs))
	}

	// Schema 0 -> 1: column id 3 (c) added, column id 2 (b -> bee) renamed.
	byKind := map[SchemaChangeKind]SchemaChange{}
	for _, c := range got.Diffs[0].Changes {
		byKind[c.Kind] = c
	}

	added, ok := byKind[ColumnAdded]
	if !ok || added.ColumnID != 3 || added.Name != "c" {
		t.Errorf("added change = %+v, want column id 3 (c)", added)
	}

	renamed, ok := byKind[ColumnRenamed]
	if !ok || renamed.ColumnID != 2 || renamed.OldName != "b" || renamed.NewName != "bee" {
		t.Errorf("renamed change = %+v, want id 2 b -> bee", renamed)
	}

	// A rename must NOT be reported as a drop + add.
	if _, dropped := byKind[ColumnDropped]; dropped {
		t.Error("rename was wrongly reported as a drop (should diff by column ID)")
	}
}
