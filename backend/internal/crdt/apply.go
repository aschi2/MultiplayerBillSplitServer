package crdt

import (
	"encoding/json"
	"time"
)

type ItemPayload struct {
	Item Item `json:"item"`
}

type ParticipantPayload struct {
	Participant Participant `json:"participant"`
}

type RemovePayload struct {
	ID string `json:"id"`
}

type AssignPayload struct {
	ItemID string `json:"item_id"`
	UserID string `json:"user_id"`
	On     bool   `json:"on"`
}

type TaxTipPayload struct {
	TaxCents int `json:"tax_cents"`
	TipCents int `json:"tip_cents"`
}

type RoomPayload struct {
	Name           string `json:"name"`
	Currency       string `json:"currency,omitempty"`
	TargetCurrency string `json:"target_currency,omitempty"`
}

func ApplyOp(doc *RoomDoc, op Op) {
	if doc == nil {
		return
	}

	if op.Timestamp == 0 {
		op.Timestamp = time.Now().UnixMilli()
	}

	// ensure maps are initialized (old snapshots may omit them)
	if doc.Items == nil {
		doc.Items = map[string]*Item{}
	}
	if doc.Participants == nil {
		doc.Participants = map[string]*Participant{}
	}
	if doc.Tombstones == nil {
		doc.Tombstones = map[string]int64{}
	}
	if doc.ParticipantTombstones == nil {
		doc.ParticipantTombstones = map[string]int64{}
	}

	switch op.Kind {
	case "set_item":
		var payload ItemPayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		item := payload.Item
		item.UpdatedAt = op.Timestamp
		if existing, ok := doc.Items[item.ID]; ok {
			if existing.UpdatedAt > op.Timestamp {
				return
			}
		}
		if doc.Tombstones[item.ID] > op.Timestamp {
			return
		}
		if item.Assigned == nil {
			item.Assigned = map[string]bool{}
		}
		doc.Items[item.ID] = &item
	case "remove_item":
		var payload RemovePayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		if payload.ID == "" {
			return
		}
		doc.Tombstones[payload.ID] = op.Timestamp
		delete(doc.Items, payload.ID)
	case "set_participant":
		var payload ParticipantPayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		participant := payload.Participant
		participant.UpdatedAt = op.Timestamp
		if doc.ParticipantTombstones[participant.ID] > op.Timestamp {
			return
		}
		if existing, ok := doc.Participants[participant.ID]; ok {
			if existing.UpdatedAt > op.Timestamp {
				return
			}
		}
		doc.Participants[participant.ID] = &participant
	case "remove_participant":
		var payload RemovePayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		if payload.ID == "" {
			return
		}
		doc.ParticipantTombstones[payload.ID] = op.Timestamp
		delete(doc.Participants, payload.ID)
	case "assign_item":
		var payload AssignPayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		item, ok := doc.Items[payload.ItemID]
		if !ok {
			return
		}
		if item.Assigned == nil {
			item.Assigned = map[string]bool{}
		}
		item.Assigned[payload.UserID] = payload.On
		item.UpdatedAt = op.Timestamp
	case "set_tax_tip":
		var payload TaxTipPayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		doc.TaxCents = payload.TaxCents
		doc.TipCents = payload.TipCents
		doc.UpdatedAt = op.Timestamp
	case "set_room_name":
		var payload RoomPayload
		if json.Unmarshal(op.Payload, &payload) != nil {
			return
		}
		if payload.Name != "" {
			doc.Name = payload.Name
			doc.UpdatedAt = op.Timestamp
		}
		if payload.Currency != "" {
			doc.Currency = payload.Currency
			doc.UpdatedAt = op.Timestamp
		}
		if payload.TargetCurrency != "" {
			doc.TargetCurrency = payload.TargetCurrency
			doc.UpdatedAt = op.Timestamp
		}
	}
}
