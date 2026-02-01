package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/aschi2/MultiplayerBillSplit/backend/internal/crdt"
	"github.com/aschi2/MultiplayerBillSplit/backend/internal/redisstore"
)

type Hub struct {
	store     *redisstore.Store
	upgrader  websocket.Upgrader
	clients   map[string]map[*websocket.Conn]bool
	connActor map[*websocket.Conn]string
	clientsMu sync.Mutex
	baseCtx   context.Context
	stopCh    chan struct{}
}

const (
	wsPongWait   = 90 * time.Second
	wsPingPeriod = 30 * time.Second
	wsWriteWait  = 10 * time.Second
)

func NewHub(store *redisstore.Store) *Hub {
	h := &Hub{
		store:     store,
		upgrader:  websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		clients:   map[string]map[*websocket.Conn]bool{},
		connActor: map[*websocket.Conn]string{},
		baseCtx:   context.Background(),
	}
	h.startPresenceLoop()
	return h
}

// loadDoc returns the latest room doc and current seq by applying ops after the stored snapshot
// and peeking at the seq key to reflect the true latest sequence.
func (h *Hub) loadDoc(ctx context.Context, roomID string) (*crdt.RoomDoc, int64) {
	room, seq, _ := h.store.LoadSnapshot(ctx, roomID)
	if room == nil {
		room = crdt.NewRoom(roomID, "")
		seq = 0
	}
	if ops, err := h.store.LoadOps(ctx, roomID, seq); err == nil {
		for _, op := range ops {
			crdt.ApplyOp(room, op)
			if op.Timestamp > room.UpdatedAt {
				room.UpdatedAt = op.Timestamp
			}
		}
	}
	if current, err := h.store.CurrentSeq(ctx, roomID); err == nil && current > seq {
		seq = current
	}
	return room, seq
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request, roomID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	// keep-alive setup
	conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	defer conn.Close()

	h.register(roomID, conn)
	actorID := ""
	defer h.handleDisconnect(roomID, conn, &actorID)

	// ping loop
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				_ = conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(wsWriteWait))
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	ctx := h.baseCtx
	room, seq := h.loadDoc(ctx, roomID)
	snapshot := map[string]any{
		"type": "snapshot",
		"seq":  seq,
		"doc":  room,
	}
	conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
	conn.WriteJSON(snapshot)

	for {
		var message struct {
			Type      string  `json:"type"`
			Op        crdt.Op `json:"op"`
			LastSeq   int64   `json:"last_seq"`
			ClientID  string  `json:"client_id"`
			Timestamp int64   `json:"timestamp"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		switch message.Type {
		case "op":
			if message.Op.ActorID != "" {
				actorID = message.Op.ActorID
				h.trackActor(roomID, conn, actorID)
			}
			if message.Op.ID == "" {
				message.Op.ID = uuid.NewString()
			}
			if message.Op.Timestamp == 0 {
				message.Op.Timestamp = time.Now().UnixMilli()
			}
			// refresh doc to latest snapshot + pending ops
			doc, _ := h.loadDoc(ctx, roomID)

			seq, err := h.store.AppendOp(ctx, roomID, message.Op)
			if err != nil {
				continue
			}
			crdt.ApplyOp(doc, message.Op)
			h.store.SaveSnapshot(ctx, roomID, doc, seq)
			h.broadcast(roomID, map[string]any{
				"type": "op",
				"seq":  seq,
				"op":   message.Op,
			})
			conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			conn.WriteJSON(map[string]any{"type": "ack", "seq": seq})
		case "resync":
			doc, currentSeq := h.loadDoc(ctx, roomID)
			conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			conn.WriteJSON(map[string]any{"type": "snapshot", "seq": currentSeq, "doc": doc})
		case "ping":
			conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			conn.WriteJSON(map[string]any{"type": "pong", "ts": time.Now().UnixMilli()})
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

func (h *Hub) snapshotPresence() map[string]map[string]bool {
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	presence := make(map[string]map[string]bool)
	for roomID, conns := range h.clients {
		for conn := range conns {
			actor := h.connActor[conn]
			if actor == "" {
				continue
			}
			if presence[roomID] == nil {
				presence[roomID] = map[string]bool{}
			}
			presence[roomID][actor] = true
		}
	}
	return presence
}

func (h *Hub) startPresenceLoop() {
	h.stopCh = make(chan struct{})
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				h.reconcilePresence()
			case <-h.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (h *Hub) reconcilePresence() {
	presence := h.snapshotPresence()
	now := time.Now().UnixMilli()

	for roomID, presentMap := range presence {
		ctx := h.baseCtx
		room, seq, err := h.store.LoadSnapshot(ctx, roomID)
		if err != nil || room == nil {
			continue
		}
		if ops, err := h.store.LoadOps(ctx, roomID, seq); err == nil {
			for _, op := range ops {
				crdt.ApplyOp(room, op)
			}
		}
		changed := false
		lastSeq := seq
		for id, participant := range room.Participants {
			desired := presentMap[id]
			if participant.Present == desired {
				continue
			}
			updated := *participant
			updated.Present = desired
			updated.UpdatedAt = now
			room.Participants[id] = &updated

			op := crdt.Op{
				ID:        uuid.NewString(),
				ActorID:   id,
				Kind:      "set_participant",
				Timestamp: now,
			}
			payload, _ := json.Marshal(crdt.ParticipantPayload{Participant: updated})
			op.Payload = payload
			if seqVal, err := h.store.AppendOp(ctx, roomID, op); err == nil {
				lastSeq = seqVal
				h.broadcast(roomID, map[string]any{"type": "op", "seq": seqVal, "op": op})
			}
			changed = true
		}
		if changed && lastSeq != 0 {
			h.store.SaveSnapshot(ctx, roomID, room, lastSeq)
		}
	}
}

func (h *Hub) trackActor(roomID string, conn *websocket.Conn, actorID string) {
	if actorID == "" {
		return
	}
	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	h.connActor[conn] = actorID
	// ensure room map exists to allow presence checks later
	if h.clients[roomID] == nil {
		h.clients[roomID] = map[*websocket.Conn]bool{}
	}
}

func (h *Hub) handleDisconnect(roomID string, conn *websocket.Conn, actorPtr *string) {
	h.clientsMu.Lock()
	actorID := ""
	if actorPtr != nil {
		actorID = *actorPtr
	}
	if actorID == "" {
		actorID = h.connActor[conn]
	}
	delete(h.connActor, conn)
	if h.clients[roomID] != nil {
		delete(h.clients[roomID], conn)
	}
	stillPresent := false
	if actorID != "" && h.clients[roomID] != nil {
		for c := range h.clients[roomID] {
			if h.connActor[c] == actorID {
				stillPresent = true
				break
			}
		}
	}
	h.clientsMu.Unlock()

	if actorID != "" && !stillPresent {
		h.markParticipantAbsent(roomID, actorID)
	}
}

func (h *Hub) markParticipantAbsent(roomID, actorID string) {
	ctx := h.baseCtx
	room, seq, err := h.store.LoadSnapshot(ctx, roomID)
	if err != nil || room == nil {
		return
	}
	if ops, err := h.store.LoadOps(ctx, roomID, seq); err == nil {
		for _, op := range ops {
			crdt.ApplyOp(room, op)
		}
	}
	participant, ok := room.Participants[actorID]
	if !ok {
		return
	}
	if !participant.Present {
		return
	}
	updated := *participant
	updated.Present = false
	updated.UpdatedAt = time.Now().UnixMilli()
	room.Participants[actorID] = &updated

	op := crdt.Op{
		ID:        uuid.NewString(),
		ActorID:   actorID,
		Kind:      "set_participant",
		Timestamp: updated.UpdatedAt,
	}
	payload, _ := json.Marshal(crdt.ParticipantPayload{Participant: updated})
	op.Payload = payload
	if seqVal, err := h.store.AppendOp(ctx, roomID, op); err == nil {
		seq = seqVal
		h.store.SaveSnapshot(ctx, roomID, room, seqVal)
		h.broadcast(roomID, map[string]any{"type": "op", "seq": seqVal, "op": op})
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
