package server

import (
	"testing"

	"github.com/aschi2/MultiplayerBillSplit/backend/internal/crdt"
)

func TestEnsureItemSortOrderBackfillsMissingValuesDeterministically(t *testing.T) {
	room := &crdt.RoomDoc{
		Items: map[string]*crdt.Item{
			"b": {
				ID:        "b",
				Name:      "Second",
				UpdatedAt: 20,
			},
			"a": {
				ID:        "a",
				Name:      "First",
				UpdatedAt: 10,
			},
		},
	}

	changed := ensureItemSortOrder(room)
	if !changed {
		t.Fatal("expected missing sort orders to be backfilled")
	}

	first := room.Items["a"]
	second := room.Items["b"]
	if first == nil || first.SortOrder == nil || second == nil || second.SortOrder == nil {
		t.Fatalf("expected sort orders to be populated, got a=%#v b=%#v", first, second)
	}
	if *first.SortOrder != 1000 {
		t.Fatalf("expected first item sort order 1000, got %d", *first.SortOrder)
	}
	if *second.SortOrder != 2000 {
		t.Fatalf("expected second item sort order 2000, got %d", *second.SortOrder)
	}
}

func TestEnsureItemSortOrderAppendsMissingAfterExistingOrders(t *testing.T) {
	existingOrder := int64(5000)
	room := &crdt.RoomDoc{
		Items: map[string]*crdt.Item{
			"existing": {
				ID:        "existing",
				Name:      "Existing",
				SortOrder: &existingOrder,
				UpdatedAt: 10,
			},
			"missing": {
				ID:        "missing",
				Name:      "Missing",
				UpdatedAt: 20,
			},
		},
	}

	changed := ensureItemSortOrder(room)
	if !changed {
		t.Fatal("expected appended sort order for missing item")
	}

	got := room.Items["missing"]
	if got == nil || got.SortOrder == nil {
		t.Fatalf("expected sort order for missing item, got %#v", got)
	}
	if *got.SortOrder != 6000 {
		t.Fatalf("expected missing item sort order 6000, got %d", *got.SortOrder)
	}
}
