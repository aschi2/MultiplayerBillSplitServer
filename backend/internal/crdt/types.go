package crdt

import (
	"encoding/json"
	"time"
)

type RoomDoc struct {
	RoomID       string                  `json:"room_id"`
	Name         string                  `json:"name"`
	Items        map[string]*Item        `json:"items"`
	Participants map[string]*Participant `json:"participants"`
	TaxCents     int                     `json:"tax_cents"`
	TipCents     int                     `json:"tip_cents"`
	Seq          int64                   `json:"seq"`
	UpdatedAt    int64                   `json:"updated_at"`
	Tombstones   map[string]int64        `json:"tombstones"`
}

type Item struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Quantity        int              `json:"quantity"`
	UnitPriceCents  int              `json:"unit_price_cents"`
	LinePriceCents  int              `json:"line_price_cents"`
	DiscountCents   int              `json:"discount_cents"`
	DiscountPercent float64          `json:"discount_percent"`
	Assigned        map[string]bool  `json:"assigned"`
	UpdatedAt       int64            `json:"updated_at"`
	RawText         string           `json:"raw_text"`
	Warnings        []string         `json:"warnings"`
	Meta            map[string]any   `json:"meta"`
}

type Participant struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Initials  string `json:"initials"`
	ColorSeed string `json:"color_seed"`
	Present   bool   `json:"present"`
	UpdatedAt int64  `json:"updated_at"`
}

type Op struct {
	ID        string          `json:"id"`
	ActorID   string          `json:"actor_id"`
	Timestamp int64           `json:"timestamp"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
}

func NewRoom(roomID, name string) *RoomDoc {
	return &RoomDoc{
		RoomID:       roomID,
		Name:         name,
		Items:        map[string]*Item{},
		Participants: map[string]*Participant{},
		Tombstones:   map[string]int64{},
		UpdatedAt:    time.Now().UnixMilli(),
	}
}
