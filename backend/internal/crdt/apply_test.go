package crdt

import (
	"encoding/json"
	"testing"
)

func TestApplyOpPreservesExistingSortOrderWhenMissingFromPayload(t *testing.T) {
	existingOrder := int64(3000)
	doc := &RoomDoc{
		Items: map[string]*Item{
			"item-1": {
				ID:        "item-1",
				Name:      "Pork Dumplings #1",
				SortOrder: &existingOrder,
			},
		},
		Tombstones:            map[string]int64{},
		ParticipantTombstones: map[string]int64{},
	}

	payload, err := json.Marshal(ItemPayload{
		Item: Item{
			ID:       "item-1",
			Name:     "Pork Dumplings #1",
			Assigned: map[string]bool{},
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	ApplyOp(doc, Op{
		Kind:      "set_item",
		Timestamp: 123,
		Payload:   payload,
	})

	got := doc.Items["item-1"]
	if got == nil || got.SortOrder == nil {
		t.Fatalf("expected preserved sort order, got %#v", got)
	}
	if *got.SortOrder != existingOrder {
		t.Fatalf("expected sort order %d, got %d", existingOrder, *got.SortOrder)
	}
}

func TestSetParticipantFinishedDefaultRed(t *testing.T) {
	doc := &RoomDoc{
		Participants:          map[string]*Participant{},
		Tombstones:            map[string]int64{},
		ParticipantTombstones: map[string]int64{},
	}

	payload, err := json.Marshal(ParticipantPayload{
		Participant: Participant{
			ID:        "user-1",
			Name:      "Alice",
			Initials:  "AL",
			ColorSeed: "aabbcc",
			Present:   true,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	ApplyOp(doc, Op{Kind: "set_participant", Timestamp: 100, Payload: payload})

	got := doc.Participants["user-1"]
	if got == nil {
		t.Fatal("expected participant to exist")
	}
	if got.Finished {
		t.Fatal("expected Finished to default to false (red border)")
	}
}

func TestSetParticipantFinishedTurnsGreen(t *testing.T) {
	doc := &RoomDoc{
		Participants:          map[string]*Participant{},
		Tombstones:            map[string]int64{},
		ParticipantTombstones: map[string]int64{},
	}

	// First, add unfinished participant
	payload1, _ := json.Marshal(ParticipantPayload{
		Participant: Participant{
			ID: "user-1", Name: "Alice", Initials: "AL", ColorSeed: "aabbcc",
		},
	})
	ApplyOp(doc, Op{Kind: "set_participant", Timestamp: 100, Payload: payload1})

	if doc.Participants["user-1"].Finished {
		t.Fatal("expected Finished=false before toggle")
	}

	// Now update with Finished=true
	payload2, _ := json.Marshal(ParticipantPayload{
		Participant: Participant{
			ID: "user-1", Name: "Alice", Initials: "AL", ColorSeed: "aabbcc", Finished: true,
		},
	})
	ApplyOp(doc, Op{Kind: "set_participant", Timestamp: 200, Payload: payload2})

	if !doc.Participants["user-1"].Finished {
		t.Fatal("expected Finished=true after toggle (green border)")
	}
}

func TestToggleFinishedOneUserDoesNotAffectAnother(t *testing.T) {
	doc := &RoomDoc{
		Participants:          map[string]*Participant{},
		Tombstones:            map[string]int64{},
		ParticipantTombstones: map[string]int64{},
	}

	// Add two participants
	for _, u := range []struct{ id, name string }{{"user-1", "Alice"}, {"user-2", "Bob"}} {
		payload, _ := json.Marshal(ParticipantPayload{
			Participant: Participant{ID: u.id, Name: u.name, Initials: u.name[:2], ColorSeed: "aabbcc"},
		})
		ApplyOp(doc, Op{Kind: "set_participant", Timestamp: 100, Payload: payload})
	}

	// Mark user-1 as finished
	payload, _ := json.Marshal(ParticipantPayload{
		Participant: Participant{
			ID: "user-1", Name: "Alice", Initials: "Al", ColorSeed: "aabbcc", Finished: true,
		},
	})
	ApplyOp(doc, Op{Kind: "set_participant", Timestamp: 200, Payload: payload})

	if !doc.Participants["user-1"].Finished {
		t.Fatal("user-1 should be finished")
	}
	if doc.Participants["user-2"].Finished {
		t.Fatal("user-2 should NOT be finished — toggling one user must not affect another")
	}
}
