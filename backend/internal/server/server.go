package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/aschi2/MultiplayerBillSplit/backend/internal/crdt"
	"github.com/aschi2/MultiplayerBillSplit/backend/internal/redisstore"
)

type Server struct {
	config Config
	hub    *Hub
	store  *redisstore.Store
}

func NewServer(config Config) (*Server, error) {
	opts, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opts)
	store := redisstore.New(client, config.RoomTTL)
	return &Server{
		config: config,
		hub:    NewHub(store),
		store:  store,
	}, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/create-room", s.handleCreateRoom)
	mux.HandleFunc("/api/join-room", s.handleJoinRoom)
	mux.HandleFunc("/api/receipt/parse", s.handleReceiptParse)
	mux.HandleFunc("/ws/", s.handleWS)
	return s.withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

type CreateRoomRequest struct {
	Name     string `json:"name"`
	BillName string `json:"bill_name"`
}

type CreateRoomResponse struct {
	RoomCode string `json:"room_code"`
	UserID   string `json:"user_id"`
	JoinToken string `json:"join_token"`
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	roomCode := randomCode(6)
	userID := uuid.NewString()
	room := crdt.NewRoom(roomCode, req.BillName)
	participant := crdt.Participant{
		ID:        userID,
		Name:      req.Name,
		Initials:  initials(req.Name),
		ColorSeed: colorSeed(roomCode, userID),
		UpdatedAt: time.Now().UnixMilli(),
	}
	room.Participants[userID] = &participant
	ctx := context.Background()
	s.store.SaveSnapshot(ctx, roomCode, room, 0)
	joinToken := s.signJoinToken(roomCode, userID)
	writeJSON(w, CreateRoomResponse{RoomCode: roomCode, UserID: userID, JoinToken: joinToken})
}

type JoinRoomRequest struct {
	RoomCode string `json:"room_code"`
	Name     string `json:"name"`
	UserID   string `json:"user_id"`
	Token    string `json:"join_token"`
}

type JoinRoomResponse struct {
	RoomCode string `json:"room_code"`
	UserID   string `json:"user_id"`
	JoinToken string `json:"join_token"`
}

func (s *Server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := context.Background()
	room, seq, err := s.store.LoadSnapshot(ctx, req.RoomCode)
	if err != nil || room == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	userID := req.UserID
	if userID == "" {
		userID = uuid.NewString()
	}
	if req.Token != "" && !s.verifyJoinToken(req.RoomCode, userID, req.Token) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	participant := crdt.Participant{
		ID:        userID,
		Name:      req.Name,
		Initials:  initials(req.Name),
		ColorSeed: colorSeed(req.RoomCode, userID),
		UpdatedAt: time.Now().UnixMilli(),
	}
	room.Participants[userID] = &participant
	s.store.SaveSnapshot(ctx, req.RoomCode, room, seq)
	joinToken := s.signJoinToken(req.RoomCode, userID)
	writeJSON(w, JoinRoomResponse{RoomCode: req.RoomCode, UserID: userID, JoinToken: joinToken})
}

func (s *Server) handleReceiptParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.config.OpenAIKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "OPENAI_API_KEY not configured"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	result, err := callOpenAIReceiptParse(r.Context(), s.config.OpenAIKey, data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimPrefix(r.URL.Path, "/ws/")
	if roomID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	s.hub.HandleWS(w, r, roomID)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && len(s.config.CorsAllowedOrigins) > 0 {
			for _, allowed := range s.config.CorsAllowedOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) signJoinToken(roomCode, userID string) string {
	payload := fmt.Sprintf("%s:%s", roomCode, userID)
	mac := hmac.New(sha256.New, []byte(s.config.JoinTokenKey))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) verifyJoinToken(roomCode, userID, token string) bool {
	if s.config.JoinTokenKey == "" {
		return true
	}
	return token == s.signJoinToken(roomCode, userID)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func randomCode(length int) string {
	letters := []rune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789")
	rand.Seed(time.Now().UnixNano())
	out := make([]rune, length)
	for i := range out {
		out[i] = letters[rand.Intn(len(letters))]
	}
	return string(out)
}

func initials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	if len(parts) == 1 {
		return strings.ToUpper(parts[0][:1])
	}
	return strings.ToUpper(parts[0][:1] + parts[len(parts)-1][:1])
}

func colorSeed(roomID, userID string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(roomID+userID)))[:6]
}
