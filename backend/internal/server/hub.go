package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"

	"github.com/aschi2/MultiplayerBillSplit/backend/internal/crdt"
	"github.com/aschi2/MultiplayerBillSplit/backend/internal/redisstore"
)

type Hub struct {
	store      *redisstore.Store
	upgrader   websocket.Upgrader
	clients    map[string]map[*websocket.Conn]bool
	clientsMu  sync.Mutex
	baseCtx    context.Context
}

func NewHub(store *redisstore.Store) *Hub {
	return &Hub{
		store:    store,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		clients:  map[string]map[*websocket.Conn]bool{},
		baseCtx:  context.Background(),
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request, roomID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	h.register(roomID, conn)
	defer h.unregister(roomID, conn)

	ctx := h.baseCtx
	room, seq, _ := h.store.LoadSnapshot(ctx, roomID)
	if room == nil {
		room = crdt.NewRoom(roomID, "")
		seq = 0
	}
	snapshot := map[string]any{
		"type": "snapshot",
		"seq":  seq,
		"doc":  room,
	}
	conn.WriteJSON(snapshot)

	for {
		var message struct {
			Type      string    `json:"type"`
			Op        crdt.Op    `json:"op"`
			LastSeq   int64      `json:"last_seq"`
			ClientID  string    `json:"client_id"`
			Timestamp int64      `json:"timestamp"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		switch message.Type {
		case "op":
			if message.Op.ID == "" {
				message.Op.ID = uuid.NewString()
			}
			if message.Op.Timestamp == 0 {
				message.Op.Timestamp = time.Now().UnixMilli()
			}
			seq, err := h.store.AppendOp(ctx, roomID, message.Op)
			if err != nil {
				continue
			}
			crdt.ApplyOp(room, message.Op)
			h.store.SaveSnapshot(ctx, roomID, room, seq)
			h.broadcast(roomID, map[string]any{
				"type": "op",
				"seq":  seq,
				"op":   message.Op,
			})
			conn.WriteJSON(map[string]any{"type": "ack", "seq": seq})
		case "resync":
			ops, _ := h.store.LoadOps(ctx, roomID, message.LastSeq)
			conn.WriteJSON(map[string]any{"type": "ops", "ops": ops})
		}
	}
}

func (h *Hub) register(roomID string, conn *websocket.Conn) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	if h.clients[roomID] == nil {
		h.clients[roomID] = map[*websocket.Conn]bool{}
	}
	h.clients[roomID][conn] = true
}

func (h *Hub) unregister(roomID string, conn *websocket.Conn) {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	if h.clients[roomID] != nil {
		delete(h.clients[roomID], conn)
	}
}

func (h *Hub) broadcast(roomID string, payload any) {
	message, _ := json.Marshal(payload)
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	for conn := range h.clients[roomID] {
		conn.WriteMessage(websocket.TextMessage, message)
	}
}
