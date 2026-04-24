package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	mux.HandleFunc("/api/room-status", s.handleRoomStatus)
	mux.HandleFunc("/api/receipt/parse", s.handleReceiptParse)
	mux.HandleFunc("/api/fx", s.handleFX)
	mux.HandleFunc("/ws/", s.handleWS)
	return s.withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

type CreateRoomRequest struct {
	Name          string `json:"name"`
	BillName      string `json:"bill_name"`
	Currency      string `json:"currency"`
	VenmoUsername string `json:"venmo_username,omitempty"`
}

type CreateRoomResponse struct {
	RoomCode       string `json:"room_code"`
	UserID         string `json:"user_id"`
	JoinToken      string `json:"join_token"`
	ColorSeed      string `json:"color_seed"`
	Currency       string `json:"currency"`
	TargetCurrency string `json:"target_currency"`
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
	if req.Currency != "" {
		room.Currency = strings.ToUpper(req.Currency)
		room.TargetCurrency = room.Currency
	}
	participant := crdt.Participant{
		ID:            userID,
		Name:          req.Name,
		Initials:      initials(req.Name),
		ColorSeed:     colorSeed(roomCode, userID),
		VenmoUsername: normalizeVenmoUsername(req.VenmoUsername),
		Present:       true,
		UpdatedAt:     time.Now().UnixMilli(),
	}
	room.Participants[userID] = &participant
	ctx := context.Background()
	s.store.SaveSnapshot(ctx, roomCode, room, 0)
	op := crdt.Op{
		ID:        uuid.NewString(),
		ActorID:   userID,
		Kind:      "set_participant",
		Timestamp: time.Now().UnixMilli(),
	}
	payload, _ := json.Marshal(crdt.ParticipantPayload{Participant: participant})
	op.Payload = payload
	seq, _ := s.store.AppendOp(ctx, roomCode, op)
	s.hub.broadcast(roomCode, map[string]any{"type": "op", "seq": seq, "op": op})

	joinToken := s.signJoinToken(roomCode, userID)
	writeJSON(w, CreateRoomResponse{
		RoomCode:       roomCode,
		UserID:         userID,
		JoinToken:      joinToken,
		ColorSeed:      participant.ColorSeed,
		Currency:       room.Currency,
		TargetCurrency: room.TargetCurrency,
	})
}

type JoinRoomRequest struct {
	RoomCode      string `json:"room_code"`
	Name          string `json:"name"`
	UserID        string `json:"user_id"`
	Token         string `json:"join_token"`
	VenmoUsername string `json:"venmo_username,omitempty"`
}

type JoinRoomResponse struct {
	RoomCode       string `json:"room_code"`
	UserID         string `json:"user_id"`
	JoinToken      string `json:"join_token"`
	ColorSeed      string `json:"color_seed"`
	Currency       string `json:"currency"`
	TargetCurrency string `json:"target_currency"`
}

type RoomStatusResponse struct {
	RoomCode       string `json:"room_code"`
	Name           string `json:"name"`
	Currency       string `json:"currency"`
	TargetCurrency string `json:"target_currency"`
	UpdatedAt      int64  `json:"updated_at"`
	ExpiresInSec   int64  `json:"expires_in_seconds"`
	TotalCents     int64  `json:"total_cents"`
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
	req.RoomCode = strings.ToUpper(strings.TrimSpace(req.RoomCode))
	if req.RoomCode == "" {
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
		// reuse participant if same name exists
		for id, p := range room.Participants {
			if strings.EqualFold(p.Name, req.Name) {
				userID = id
				break
			}
		}
		if userID == "" {
			userID = uuid.NewString()
		}
	}
	if req.Token != "" && !s.verifyJoinToken(req.RoomCode, userID, req.Token) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var participant crdt.Participant
	normalizedVenmo := normalizeVenmoUsername(req.VenmoUsername)
	if existing, ok := room.Participants[userID]; ok {
		participant = *existing
		participant.Name = req.Name
		if normalizedVenmo != "" {
			participant.VenmoUsername = normalizedVenmo
		}
		participant.Present = true
		participant.UpdatedAt = time.Now().UnixMilli()
	} else {
		participant = crdt.Participant{
			ID:            userID,
			Name:          req.Name,
			Initials:      initials(req.Name),
			ColorSeed:     colorSeed(req.RoomCode, userID),
			VenmoUsername: normalizedVenmo,
			Present:       true,
			UpdatedAt:     time.Now().UnixMilli(),
		}
	}
	room.Participants[userID] = &participant
	s.store.SaveSnapshot(ctx, req.RoomCode, room, seq)
	op := crdt.Op{
		ID:        uuid.NewString(),
		ActorID:   userID,
		Kind:      "set_participant",
		Timestamp: time.Now().UnixMilli(),
	}
	payload, _ := json.Marshal(crdt.ParticipantPayload{Participant: participant})
	op.Payload = payload
	newSeq, _ := s.store.AppendOp(ctx, req.RoomCode, op)
	s.hub.broadcast(req.RoomCode, map[string]any{"type": "op", "seq": newSeq, "op": op})

	joinToken := s.signJoinToken(req.RoomCode, userID)
	writeJSON(w, JoinRoomResponse{
		RoomCode:       req.RoomCode,
		UserID:         userID,
		JoinToken:      joinToken,
		ColorSeed:      participant.ColorSeed,
		Currency:       room.Currency,
		TargetCurrency: room.TargetCurrency,
	})
}

func (s *Server) handleRoomStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	roomCode := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("room_code")))
	if roomCode == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := context.Background()
	room, _, err := s.store.LoadSnapshot(ctx, roomCode)
	if err != nil || room == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	ttl, ttlErr := s.store.SnapshotTTL(ctx, roomCode)
	if ttlErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	expiresInSec := int64(0)
	if ttl > 0 {
		expiresInSec = int64((ttl + time.Second - 1) / time.Second)
	}
	writeJSON(w, RoomStatusResponse{
		RoomCode:       roomCode,
		Name:           room.Name,
		Currency:       room.Currency,
		TargetCurrency: room.TargetCurrency,
		UpdatedAt:      room.UpdatedAt,
		ExpiresInSec:   expiresInSec,
		TotalCents:     computeRoomTotalCents(room),
	})
}

func computeRoomTotalCents(room *crdt.RoomDoc) int64 {
	if room == nil {
		return 0
	}
	gross := int64(0)
	itemDiscount := int64(0)
	for _, it := range room.Items {
		if it == nil {
			continue
		}
		line := int64(it.LinePriceCents)
		if line < 0 {
			line = 0
		}
		qty := int64(it.Quantity)
		if qty <= 0 {
			qty = 1
		}
		disc := int64(it.DiscountCents) * qty
		if disc < 0 {
			disc = 0
		}
		gross += line
		itemDiscount += disc
	}
	if itemDiscount > gross {
		itemDiscount = gross
	}
	billDiscount := int64(room.BillDiscountCents)
	if billDiscount < 0 {
		billDiscount = 0
	}
	maxBillDiscount := gross - itemDiscount
	if billDiscount > maxBillDiscount {
		billDiscount = maxBillDiscount
	}
	net := gross - itemDiscount - billDiscount
	if net < 0 {
		net = 0
	}
	billCharges := int64(room.BillChargesCents)
	if billCharges < 0 {
		billCharges = 0
	}
	tax := int64(room.TaxCents)
	if tax < 0 {
		tax = 0
	}
	tip := int64(room.TipCents)
	if tip < 0 {
		tip = 0
	}
	return net + billCharges + tax + tip
}

func (s *Server) handleReceiptParse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.config.GeminiKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "Gemini API key is not configured"})
		return
	}
	file, header, err := r.FormFile("file")
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
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "Unsupported file type. Please upload an image."})
		return
	}
	switch contentType {
	case "image/heic", "image/heif":
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": "HEIC images aren't supported yet. Please upload a JPEG or PNG."})
		return
	}
	// Keep a single model call, but normalize orientation first so dense journal-style
	// screenshots are sent in the most readable rotation. Skip when the client signals
	// the user already cropped/rotated -- otherwise the heuristic crop can strip
	// content the user deliberately included.
	userCropped := strings.EqualFold(strings.TrimSpace(r.FormValue("user_cropped")), "1") ||
		strings.EqualFold(strings.TrimSpace(r.FormValue("user_cropped")), "true")
	if !userCropped {
		if normalizedData, normalizedType, rotated := normalizeReceiptImageOrientation(data, contentType); rotated {
			data = normalizedData
			contentType = normalizedType
		}
	}
	parseMode := strings.ToLower(strings.TrimSpace(r.FormValue("parse_mode")))
	preferHighAccuracy := parseMode == "accurate" || parseMode == "retry" || parseMode == "high"
	parseTemperature := geminiReceiptTemperatureStandard
	primaryModel := geminiModelPrimary
	fallbackModel := geminiModelFallback
	if preferHighAccuracy {
		parseTemperature = geminiReceiptTemperatureRetry
		primaryModel = geminiModelRetryPrimary
		fallbackModel = geminiModelRetryFallback
	}

	result, err := callGeminiReceiptParseWithModel(
		r.Context(),
		s.config.GeminiKey,
		data,
		contentType,
		primaryModel,
		parseTemperature,
	)
	if err != nil {
		primaryErr := err
		if fallbackModel == "" || strings.EqualFold(primaryModel, fallbackModel) {
			err = fmt.Errorf("%s failed (%v)", primaryModel, primaryErr)
		} else {
			result, err = callGeminiReceiptParseWithModel(
				r.Context(),
				s.config.GeminiKey,
				data,
				contentType,
				fallbackModel,
				parseTemperature,
			)
			if err == nil {
				if preferHighAccuracy {
					result.Warnings = append(result.Warnings, "Try-again parse fallback used.")
				} else {
					result.Warnings = append(result.Warnings, "Primary Gemini parse failed; fallback model used.")
				}
				log.Printf("receipt parse fallback: %s failed (%v), %s succeeded", primaryModel, primaryErr, fallbackModel)
			} else {
				err = fmt.Errorf("%s failed (%v); %s fallback failed (%v)", primaryModel, primaryErr, fallbackModel, err)
			}
		}
	}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"error": err.Error()})
		return
	}
	normalizeReceiptParseResult(result)
	if shouldRunModifierTagging(result, preferHighAccuracy) {
		tags, tagErr := callGeminiModifierTagging(
			r.Context(),
			s.config.GeminiKey,
			primaryModel,
			fallbackModel,
			result,
		)
		if tagErr != nil {
			log.Printf("receipt modifier-tagging skipped: %v", tagErr)
		} else {
			merged := consolidateTaggedModifierRows(result, tags)
			if merged > 0 {
				appendReceiptWarningUnique(
					result,
					fmt.Sprintf("Consolidated %d likely modifier row%s into add-ons.", merged, map[bool]string{true: "", false: "s"}[merged == 1]),
				)
				normalizeReceiptParseResult(result)
			}
		}
	}
	writeJSON(w, result)
}

var moneyTokenPattern = regexp.MustCompile(`\d{1,3}(?:,\d{3})*(?:\.\d{2})|\d+\.\d{2}|\d{3,}(?:,\d{3})*`)
var fallbackLineSplitPattern = regexp.MustCompile(`[\r\n|]+`)
var fallbackGuestNumberPrefixPattern = regexp.MustCompile(`(?i)^\s*(?:guest\s*(?:number|#)?\s*\d+\s*)+`)
var fallbackGuestWordPrefixPattern = regexp.MustCompile(`(?i)^\s*guest\s+`)
var fallbackSeatPrefixPattern = regexp.MustCompile(`(?i)^\s*(?:seat|table)\s*#?\s*\d+\s*`)
var fallbackLeadingCodePattern = regexp.MustCompile(`^\s*\$?\d+\s+`)
var fallbackLeadingQtyPattern = regexp.MustCompile(`^\s*(\d{1,2})\s+(.+)$`)
var trailingCurrencySymbolPattern = regexp.MustCompile(`\s*[$€£¥₩₹]+\s*$`)
var leadingMultiplierPattern = regexp.MustCompile(`(?i)^\s*(?:\d+\s*x|x\s*\d+)\s+`)

var fallbackNonItemKeywords = []string{
	"subtotal",
	"total",
	"tax",
	"tip",
	"gratuity",
	"amount due",
	"payment",
	"change due",
	"balance due",
	"guest count",
	"check closed",
	"account number",
	"auth code",
	"trace no",
	"american express",
	"visa",
	"mastercard",
	"thank you",
	"download",
	"mobile order",
	"location",
}

var beverageKeywords = []string{
	"coke",
	"cola",
	"soda",
	"pepsi",
	"sprite",
	"fanta",
	"dr pepper",
	"coffee",
	"tea",
	"espresso",
	"latte",
	"cappuccino",
	"americano",
	"beer",
	"lager",
	"ipa",
	"ale",
	"stout",
	"pilsner",
	"wine",
	"cabernet",
	"chardonnay",
	"sauvignon",
	"pinot",
	"merlot",
	"cocktail",
	"whiskey",
	"bourbon",
	"vodka",
	"gin",
	"rum",
	"tequila",
	"juice",
	"water",
	"sparkling",
	"sapporo",
}

func normalizeReceiptParseResult(result *ReceiptParseResult) {
	if result == nil {
		return
	}
	backfillSparseReceiptItems(result)
	attachStandaloneAppOnlyRows(result)
	extractZeroPriceAddonLinesFromRawText(result)
	for i := range result.Items {
		item := &result.Items[i]
		item.Name = normalizeReceiptLabel(item.Name)
		if item.Name == "" {
			item.Name = normalizeReceiptLabel(ptrString(item.RawText))
		}
		for j := range item.Addons {
			addon := &item.Addons[j]
			addon.Name = normalizeReceiptLabel(addon.Name)
			if addon.Name == "" {
				addon.Name = normalizeReceiptLabel(ptrString(addon.RawText))
			}
		}
		normalizeReceiptItemPricing(item)
	}
	normalizeAddonBasePricing(result)
	repairItemLinePricesAgainstSubtotal(result)
	normalizeSubtotalFromTotalAndTax(result)
}

func shouldRunModifierTagging(result *ReceiptParseResult, preferHighAccuracy bool) bool {
	if result == nil || len(result.Items) < 4 {
		return false
	}
	addonCount := receiptAddonCount(result.Items)
	if preferHighAccuracy {
		return true
	}
	// In standard mode, still run tagging on moderately dense parses that have no
	// structured add-ons yet; waiting for 10+ rows was too conservative.
	if len(result.Items) >= 6 && addonCount == 0 {
		return true
	}
	// Some receipts include one early attached row (for example app-only condiment)
	// while still leaving many standalone modifier rows. Keep tagging enabled for
	// dense parses with only a small number of pre-existing add-ons.
	return len(result.Items) >= 10 && addonCount <= 2
}

func consolidateTaggedModifierRows(result *ReceiptParseResult, tags []ReceiptModifierTag) int {
	if result == nil || len(result.Items) < 2 || len(tags) == 0 {
		return 0
	}
	itemCount := len(result.Items)
	tagByIndex := make(map[int]ReceiptModifierTag, len(tags))
	for _, tag := range tags {
		if tag.Index < 0 || tag.Index >= itemCount {
			continue
		}
		tagByIndex[tag.Index] = tag
	}

	targetByModifier := make(map[int]int, itemCount/2)
	for idx, tag := range tagByIndex {
		if strings.ToLower(strings.TrimSpace(tag.Role)) != "modifier" {
			continue
		}
		if tag.Confidence > 0 && tag.Confidence < 0.55 {
			continue
		}
		if tag.TargetIndex == nil {
			continue
		}
		target := *tag.TargetIndex
		if target < 0 || target >= itemCount || target >= idx {
			continue
		}
		targetByModifier[idx] = target
	}
	if len(targetByModifier) == 0 {
		return 0
	}

	resolveTarget := func(modifierIndex int) int {
		visited := map[int]struct{}{}
		target, ok := targetByModifier[modifierIndex]
		if !ok {
			return -1
		}
		for {
			if target < 0 || target >= itemCount {
				return -1
			}
			if _, seen := visited[target]; seen {
				return -1
			}
			visited[target] = struct{}{}
			next, nextIsModifier := targetByModifier[target]
			if !nextIsModifier {
				return target
			}
			if next >= target {
				return -1
			}
			target = next
		}
	}

	attachments := make(map[int][]int, itemCount/2)
	for modifierIndex := range targetByModifier {
		target := resolveTarget(modifierIndex)
		if target < 0 || target >= modifierIndex {
			continue
		}
		attachments[target] = append(attachments[target], modifierIndex)
	}
	if len(attachments) == 0 {
		return 0
	}

	isModifier := make([]bool, itemCount)
	for _, modifierRows := range attachments {
		for _, modifierIndex := range modifierRows {
			if modifierIndex >= 0 && modifierIndex < itemCount {
				isModifier[modifierIndex] = true
			}
		}
	}

	newItems := make([]ReceiptItem, 0, itemCount)
	oldToNew := make(map[int]int, itemCount)
	for idx, item := range result.Items {
		if isModifier[idx] {
			continue
		}
		oldToNew[idx] = len(newItems)
		newItems = append(newItems, item)
	}

	merged := 0
	for baseOldIndex, modifierRows := range attachments {
		baseNewIndex, ok := oldToNew[baseOldIndex]
		if !ok || baseNewIndex < 0 || baseNewIndex >= len(newItems) {
			continue
		}
		base := &newItems[baseNewIndex]
		baseLine := receiptItemLineCents(*base)
		baseQty := receiptItemQuantity(*base)
		for _, modifierIndex := range modifierRows {
			if modifierIndex < 0 || modifierIndex >= len(result.Items) {
				continue
			}
			modifier := result.Items[modifierIndex]
			addonName := strings.TrimSpace(modifier.Name)
			if addonName == "" {
				addonName = strings.TrimSpace(ptrString(modifier.RawText))
			}
			if addonName == "" {
				addonName = fmt.Sprintf("Modifier %d", modifierIndex+1)
			}
			addonName = normalizeReceiptLabel(leadingMultiplierPattern.ReplaceAllString(addonName, ""))
			modifierLine := receiptItemLineCents(modifier)
			addonPrice := modifierLine
			if modifier.Quantity != nil && *modifier.Quantity > 1 && modifier.UnitPriceCents != nil && *modifier.UnitPriceCents > 0 {
				expected := int(math.Round(float64(*modifier.UnitPriceCents) * *modifier.Quantity))
				if modifierLine == expected {
					addonPrice = *modifier.UnitPriceCents
				}
			}
			addon := ReceiptAddon{
				Name:       addonName,
				PriceCents: intPtr(addonPrice),
				RawText:    modifier.RawText,
			}
			base.Addons = append(base.Addons, addon)
			if modifierLine > 0 {
				baseLine += modifierLine
			}
			modRaw := strings.TrimSpace(ptrString(modifier.RawText))
			if modRaw != "" {
				baseRaw := strings.TrimSpace(ptrString(base.RawText))
				if baseRaw == "" {
					rawCopy := modRaw
					base.RawText = &rawCopy
				} else if !strings.Contains(baseRaw, modRaw) {
					joined := baseRaw + "\n" + modRaw
					base.RawText = &joined
				}
			}
			merged++
		}
		if baseLine > 0 {
			base.LinePriceCents = intPtr(baseLine)
			if baseQty <= 0 {
				baseQty = 1
			}
			perUnit := int(math.Round(float64(baseLine) / baseQty))
			if perUnit < 0 {
				perUnit = 0
			}
			base.UnitPriceCents = intPtr(perUnit)
		}
	}

	if merged > 0 {
		result.Items = newItems
	}
	return merged
}

func appendReceiptWarningUnique(result *ReceiptParseResult, warning string) {
	if result == nil {
		return
	}
	clean := strings.TrimSpace(warning)
	if clean == "" {
		return
	}
	for _, existing := range result.Warnings {
		if strings.TrimSpace(existing) == clean {
			return
		}
	}
	result.Warnings = append(result.Warnings, clean)
}

func receiptItemQuantity(item ReceiptItem) float64 {
	if item.Quantity != nil && *item.Quantity > 0 {
		return *item.Quantity
	}
	return 1
}

func receiptItemLineCents(item ReceiptItem) int {
	if item.LinePriceCents != nil && *item.LinePriceCents >= 0 {
		return *item.LinePriceCents
	}
	if item.UnitPriceCents != nil && *item.UnitPriceCents >= 0 {
		return int(math.Round(float64(*item.UnitPriceCents) * receiptItemQuantity(item)))
	}
	return 0
}

func receiptItemNetCents(item ReceiptItem) int {
	line := receiptItemLineCents(item)
	if line <= 0 {
		return 0
	}
	discount := 0
	if item.DiscountCents != nil && *item.DiscountCents > 0 {
		discount = int(math.Round(float64(*item.DiscountCents) * receiptItemQuantity(item)))
	}
	net := line - discount
	if net < 0 {
		return 0
	}
	return net
}

func receiptItemsNetSubtotal(items []ReceiptItem) int {
	total := 0
	for _, item := range items {
		total += receiptItemNetCents(item)
	}
	if total < 0 {
		return 0
	}
	return total
}

func receiptBillDiscountCents(result *ReceiptParseResult) int {
	if result == nil || result.BillDiscountCents == nil || *result.BillDiscountCents <= 0 {
		return 0
	}
	return *result.BillDiscountCents
}

func receiptBillChargesCents(result *ReceiptParseResult) int {
	if result == nil || result.BillChargesCents == nil || *result.BillChargesCents <= 0 {
		return 0
	}
	return *result.BillChargesCents
}

func backfillSparseReceiptItems(result *ReceiptParseResult) {
	if result == nil {
		return
	}
	existingCount := len(result.Items)
	// Do not override parses that already look reasonably structured.
	// Backfill is only for sparse outputs (e.g., first-item-only failures).
	if existingCount >= 5 || receiptAddonCount(result.Items) > 0 {
		return
	}
	lines := collectFallbackReceiptLines(result)
	if len(lines) == 0 {
		return
	}
	recovered := parseFallbackItemsFromLines(lines)
	if len(recovered) < 2 {
		return
	}
	if existingCount >= 2 {
		minRecovered := existingCount + 2
		if existingCount <= 3 {
			minRecovered = existingCount + 1
		}
		if len(recovered) < minRecovered {
			return
		}
	}
	result.Items = recovered
	warning := fmt.Sprintf(
		"Recovered %d item lines from dense receipt text (replaced %d sparse parsed items).",
		len(recovered),
		existingCount,
	)
	for _, existing := range result.Warnings {
		if existing == warning {
			return
		}
	}
	result.Warnings = append(result.Warnings, warning)
}

func receiptAddonCount(items []ReceiptItem) int {
	total := 0
	for _, item := range items {
		total += len(item.Addons)
	}
	return total
}

func attachStandaloneAppOnlyRows(result *ReceiptParseResult) {
	if result == nil || len(result.Items) < 2 {
		return
	}
	attached := 0
	nextItems := make([]ReceiptItem, 0, len(result.Items))
	for _, item := range result.Items {
		if len(nextItems) == 0 || !isStandaloneAppOnlyRow(item) {
			nextItems = append(nextItems, item)
			continue
		}
		parent := &nextItems[len(nextItems)-1]
		addonName := normalizeReceiptLabel(item.Name)
		if addonName == "" {
			addonName = normalizeReceiptLabel(ptrString(item.RawText))
		}
		if addonName == "" {
			nextItems = append(nextItems, item)
			continue
		}
		addonPrice := receiptItemLineCents(item)
		if addonPrice < 0 {
			addonPrice = 0
		}
		addon := ReceiptAddon{
			Name:    addonName,
			RawText: item.RawText,
		}
		if addonPrice > 0 {
			addon.PriceCents = intPtr(addonPrice)
		}
		duplicate := false
		for _, existing := range parent.Addons {
			if strings.EqualFold(strings.TrimSpace(existing.Name), strings.TrimSpace(addon.Name)) {
				existingPrice := 0
				if existing.PriceCents != nil {
					existingPrice = *existing.PriceCents
				}
				if existingPrice == addonPrice {
					duplicate = true
					break
				}
			}
		}
		if duplicate {
			continue
		}
		parent.Addons = append(parent.Addons, addon)
		attached++
	}
	if attached > 0 {
		result.Items = nextItems
		appendReceiptWarningUnique(result, fmt.Sprintf("Attached %d app-only modifier row%s to parent item%s.", attached, map[bool]string{true: "", false: "s"}[attached == 1], map[bool]string{true: "", false: "s"}[attached == 1]))
	}
}

func isStandaloneAppOnlyRow(item ReceiptItem) bool {
	if len(item.Addons) > 0 {
		return false
	}
	qty := receiptItemQuantity(item)
	if qty > 1.0001 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(item.Name + " " + ptrString(item.RawText)))
	if text == "" {
		return false
	}
	if looksLikeBeverageLine(text) {
		return false
	}
	return strings.Contains(text, "app only")
}

func extractZeroPriceAddonLinesFromRawText(result *ReceiptParseResult) {
	if result == nil || len(result.Items) == 0 {
		return
	}
	added := 0
	for idx := range result.Items {
		item := &result.Items[idx]
		raw := strings.TrimSpace(ptrString(item.RawText))
		if raw == "" || !strings.Contains(raw, "\n") {
			continue
		}
		lines := fallbackLineSplitPattern.Split(raw, -1)
		if len(lines) <= 1 {
			continue
		}
		existingByName := map[string]struct{}{}
		for _, addon := range item.Addons {
			key := canonicalAddonNameForDedupe(addon.Name)
			if key != "" {
				existingByName[key] = struct{}{}
			}
		}
		for _, rawLine := range lines[1:] {
			line := strings.Join(strings.Fields(strings.TrimSpace(rawLine)), " ")
			if line == "" {
				continue
			}
			lower := strings.ToLower(line)
			if fallbackShouldSkipLine(lower) {
				continue
			}
			if looksLikeBeverageLine(lower) {
				continue
			}
			lineToken, lineCents, hasMoney := rightmostMoneyToken(line)
			zeroPriced := hasMoney && lineCents == 0
			modifierText := strings.Contains(lower, " no ") || strings.HasPrefix(lower, "no ") || strings.Contains(lower, " without ")
			if !zeroPriced && !modifierText {
				continue
			}
			namePart := line
			if hasMoney {
				if cut := strings.LastIndex(namePart, lineToken); cut >= 0 {
					namePart = strings.TrimSpace(namePart[:cut])
				}
			}
			name := normalizeReceiptLabel(namePart)
			if name == "" {
				continue
			}
			nameLower := strings.ToLower(name)
			if strings.Contains(nameLower, "cold") {
				parentLower := strings.ToLower(item.Name + " " + ptrString(item.RawText))
				if !(strings.Contains(parentLower, "pie") || strings.Contains(parentLower, "cake") || strings.Contains(parentLower, "dessert")) {
					continue
				}
			}
			if name == item.Name {
				continue
			}
			key := canonicalAddonNameForDedupe(name)
			if _, exists := existingByName[key]; exists {
				continue
			}
			price := 0
			addon := ReceiptAddon{
				Name:       name,
				PriceCents: &price,
				RawText:    &line,
			}
			item.Addons = append(item.Addons, addon)
			existingByName[key] = struct{}{}
			added++
		}
	}
	if added > 0 {
		appendReceiptWarningUnique(
			result,
			fmt.Sprintf("Recovered %d zero-price modifier row%s from raw item text.", added, map[bool]string{true: "", false: "s"}[added == 1]),
		)
	}
}

func canonicalAddonNameForDedupe(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if token, _, ok := rightmostMoneyToken(trimmed); ok {
		if idx := strings.LastIndex(trimmed, token); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
	}
	return strings.ToLower(strings.TrimSpace(normalizeReceiptLabel(trimmed)))
}

func normalizeAddonBasePricing(result *ReceiptParseResult) {
	if result == nil {
		return
	}
	for idx := range result.Items {
		item := &result.Items[idx]
		if len(item.Addons) == 0 {
			continue
		}
		raw := ptrString(item.RawText)
		if !strings.Contains(raw, "\n") {
			continue
		}
		line := receiptItemLineCents(*item)
		if line <= 0 {
			continue
		}
		addonTotal := 0
		for _, addon := range item.Addons {
			if addon.PriceCents != nil && *addon.PriceCents > 0 {
				addonTotal += *addon.PriceCents
			}
		}
		if addonTotal <= 0 || addonTotal >= line {
			continue
		}
		baseLine := line - addonTotal
		if baseLine <= 0 {
			continue
		}
		item.LinePriceCents = intPtr(baseLine)
		qty := receiptItemQuantity(*item)
		if qty <= 0 {
			qty = 1
		}
		baseUnit := int(math.Round(float64(baseLine) / qty))
		if baseUnit < 0 {
			baseUnit = 0
		}
		item.UnitPriceCents = intPtr(baseUnit)
	}
}

func receiptParseNeedsQualityFallback(result *ReceiptParseResult) bool {
	if result == nil {
		return true
	}
	itemCount := len(result.Items)
	if itemCount == 0 {
		return true
	}
	nonEmptyNames := 0
	pricedLines := 0
	for _, item := range result.Items {
		if strings.TrimSpace(item.Name) != "" {
			nonEmptyNames++
		}
		if receiptItemLineCents(item) > 0 {
			pricedLines++
		}
	}
	hasAnyTotals := result.SubtotalCents != nil || result.TaxCents != nil || result.TotalCents != nil
	if itemCount <= 3 {
		// For tiny parses, require stronger evidence that output is complete.
		if !hasAnyTotals {
			return true
		}
		if nonEmptyNames < itemCount || pricedLines < maxInt(1, itemCount-1) {
			return true
		}
		return false
	}
	if nonEmptyNames < itemCount-1 {
		return true
	}
	if pricedLines < maxInt(2, itemCount/2) {
		return true
	}
	return false
}

func receiptParseQualityScore(result *ReceiptParseResult) int {
	if result == nil {
		return -1
	}
	itemCount := len(result.Items)
	nonEmptyNames := 0
	pricedLines := 0
	for _, item := range result.Items {
		if strings.TrimSpace(item.Name) != "" {
			nonEmptyNames++
		}
		if receiptItemLineCents(item) > 0 {
			pricedLines++
		}
	}
	score := 0
	score += minInt(itemCount, 20) * 5
	score += nonEmptyNames * 4
	score += pricedLines * 6
	score += receiptAddonCount(result.Items) * 3
	if result.SubtotalCents != nil {
		score += 8
	}
	if result.TaxCents != nil {
		score += 4
	}
	if result.TotalCents != nil {
		score += 8
	}
	if strings.TrimSpace(result.Currency) != "" {
		score += 2
	}
	if nonEmptyNames*2 < maxInt(1, itemCount) {
		score -= 20
	}
	score -= len(result.Warnings) * 2
	return score
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeReceiptImageOrientation(data []byte, contentType string) ([]byte, string, bool) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, contentType, false
	}
	bounds := img.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth < 200 || sourceHeight < 200 {
		return data, contentType, false
	}

	processed := false
	if rotated, ok := rotateReceiptImageToBestQuarterTurn(img); ok {
		img = rotated
		processed = true
	}
	if cropped, ok := cropReceiptPaperRegion(img); ok {
		img = cropped
		processed = true
	}
	if !processed {
		return data, contentType, false
	}
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, img); err != nil {
		return data, contentType, false
	}
	return buf.Bytes(), "image/png", true
}

func rotateReceiptImageToBestQuarterTurn(img image.Image) (image.Image, bool) {
	grayscale, sampleWidth, sampleHeight := sampleImageLuma(img, 1200)
	if sampleWidth < 120 || sampleHeight < 120 {
		return nil, false
	}
	threshold := adaptiveDarkThreshold(grayscale)
	baseScore := textOrientationScore(grayscale, sampleWidth, sampleHeight, threshold, 0)
	bestAngle := 0
	bestScore := baseScore
	for _, angle := range []int{90, 180, 270} {
		score := textOrientationScore(grayscale, sampleWidth, sampleHeight, threshold, angle)
		if score > bestScore {
			bestScore = score
			bestAngle = angle
		}
	}
	// Require a meaningful margin before rotating to avoid unnecessary flips.
	if bestAngle == 0 || bestScore < baseScore+0.08 {
		return nil, false
	}
	rotated := rotateImageQuarterTurns(img, bestAngle)
	if rotated == nil {
		return nil, false
	}
	return rotated, true
}

func cropReceiptPaperRegion(img image.Image) (image.Image, bool) {
	bounds := img.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth < 300 || sourceHeight < 300 {
		return nil, false
	}

	luma, chroma, sampleWidth, sampleHeight := sampleImageLumaAndChroma(img, 900)
	if sampleWidth < 120 || sampleHeight < 120 {
		return nil, false
	}
	lumaThreshold := adaptiveBrightThreshold(luma)
	chromaThreshold := adaptivePaperChromaThreshold(chroma)
	mask := make([]bool, len(luma))
	trueCount := 0
	for i := range luma {
		isBrightNeutral := luma[i] >= lumaThreshold && chroma[i] <= chromaThreshold
		isVeryBright := luma[i] >= 232 && chroma[i] <= chromaThreshold+12
		if isBrightNeutral || isVeryBright {
			mask[i] = true
			trueCount++
		}
	}
	if trueCount == 0 {
		return nil, false
	}

	minX, minY, maxX, maxY, area, ok := largestMaskComponentBounds(mask, sampleWidth, sampleHeight)
	if !ok {
		return nil, false
	}
	if qMinX, qMinY, qMaxX, qMaxY, ok := maskQuantileBounds(mask, sampleWidth, sampleHeight, 0.04, 0.96); ok {
		minX = maxInt(minX, qMinX)
		minY = maxInt(minY, qMinY)
		maxX = minInt(maxX, qMaxX)
		maxY = minInt(maxY, qMaxY)
	}
	sampleArea := sampleWidth * sampleHeight
	if area < sampleArea/40 {
		return nil, false
	}
	componentWidth := maxX - minX + 1
	componentHeight := maxY - minY + 1
	if componentWidth <= 0 || componentHeight <= 0 {
		return nil, false
	}
	bboxArea := componentWidth * componentHeight
	fillRatio := float64(area) / float64(maxInt(1, bboxArea))
	if fillRatio < 0.12 {
		return nil, false
	}
	cropRatio := float64(bboxArea) / float64(sampleArea)
	if cropRatio <= 0.05 || cropRatio >= 0.96 {
		return nil, false
	}
	minX, minY, maxX, maxY = tightenMaskBounds(mask, sampleWidth, sampleHeight, minX, minY, maxX, maxY)
	componentWidth = maxX - minX + 1
	componentHeight = maxY - minY + 1
	if componentWidth <= 0 || componentHeight <= 0 {
		return nil, false
	}

	scaleX := float64(sourceWidth) / float64(sampleWidth)
	scaleY := float64(sourceHeight) / float64(sampleHeight)
	origMinX := int(math.Floor(float64(minX) * scaleX))
	origMinY := int(math.Floor(float64(minY) * scaleY))
	origMaxX := int(math.Ceil(float64(maxX+1) * scaleX))
	origMaxY := int(math.Ceil(float64(maxY+1) * scaleY))

	marginX := maxInt(24, int(math.Round(float64(origMaxX-origMinX)*0.06)))
	marginY := maxInt(24, int(math.Round(float64(origMaxY-origMinY)*0.06)))
	origMinX = maxInt(0, origMinX-marginX)
	origMinY = maxInt(0, origMinY-marginY)
	origMaxX = minInt(sourceWidth, origMaxX+marginX)
	origMaxY = minInt(sourceHeight, origMaxY+marginY)
	if origMaxX-origMinX < sourceWidth/4 || origMaxY-origMinY < sourceHeight/4 {
		return nil, false
	}
	if (origMaxX-origMinX)*(origMaxY-origMinY) >= (sourceWidth*sourceHeight*97)/100 {
		return nil, false
	}

	cropRect := image.Rect(
		bounds.Min.X+origMinX,
		bounds.Min.Y+origMinY,
		bounds.Min.X+origMaxX,
		bounds.Min.Y+origMaxY,
	)
	return copyImageRect(img, cropRect), true
}

func tightenMaskBounds(mask []bool, width, height, minX, minY, maxX, maxY int) (int, int, int, int) {
	if len(mask) != width*height || width <= 0 || height <= 0 {
		return minX, minY, maxX, maxY
	}
	for minX < maxX {
		count := 0
		for y := minY; y <= maxY; y++ {
			if mask[y*width+minX] {
				count++
			}
		}
		if count >= maxInt(4, (maxY-minY+1)/18) {
			break
		}
		minX++
	}
	for maxX > minX {
		count := 0
		for y := minY; y <= maxY; y++ {
			if mask[y*width+maxX] {
				count++
			}
		}
		if count >= maxInt(4, (maxY-minY+1)/18) {
			break
		}
		maxX--
	}
	for minY < maxY {
		count := 0
		for x := minX; x <= maxX; x++ {
			if mask[minY*width+x] {
				count++
			}
		}
		if count >= maxInt(4, (maxX-minX+1)/18) {
			break
		}
		minY++
	}
	for maxY > minY {
		count := 0
		for x := minX; x <= maxX; x++ {
			if mask[maxY*width+x] {
				count++
			}
		}
		if count >= maxInt(4, (maxX-minX+1)/18) {
			break
		}
		maxY--
	}
	return minX, minY, maxX, maxY
}

func maskQuantileBounds(mask []bool, width, height int, lowerFrac, upperFrac float64) (int, int, int, int, bool) {
	if len(mask) != width*height || width <= 0 || height <= 0 {
		return 0, 0, 0, 0, false
	}
	if lowerFrac < 0 {
		lowerFrac = 0
	}
	if upperFrac > 1 {
		upperFrac = 1
	}
	if lowerFrac >= upperFrac {
		return 0, 0, 0, 0, false
	}
	colCounts := make([]int, width)
	rowCounts := make([]int, height)
	total := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if !mask[y*width+x] {
				continue
			}
			colCounts[x]++
			rowCounts[y]++
			total++
		}
	}
	if total == 0 {
		return 0, 0, 0, 0, false
	}
	lowerCount := int(math.Floor(float64(total) * lowerFrac))
	upperCount := int(math.Ceil(float64(total) * upperFrac))
	minX, maxX, okX := quantileIndexBounds(colCounts, lowerCount, upperCount)
	minY, maxY, okY := quantileIndexBounds(rowCounts, lowerCount, upperCount)
	if !okX || !okY || minX >= maxX || minY >= maxY {
		return 0, 0, 0, 0, false
	}
	return minX, minY, maxX, maxY, true
}

func quantileIndexBounds(counts []int, lowerCount, upperCount int) (int, int, bool) {
	if len(counts) == 0 {
		return 0, 0, false
	}
	running := 0
	minIndex := -1
	maxIndex := -1
	for idx, count := range counts {
		if count <= 0 {
			continue
		}
		if minIndex < 0 && running+count > lowerCount {
			minIndex = idx
		}
		running += count
		if maxIndex < 0 && running >= upperCount {
			maxIndex = idx
			break
		}
	}
	if minIndex < 0 {
		return 0, 0, false
	}
	if maxIndex < 0 {
		maxIndex = len(counts) - 1
	}
	return minIndex, maxIndex, true
}

func sampleImageLuma(img image.Image, maxDimension int) ([]uint8, int, int) {
	bounds := img.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	targetWidth := sourceWidth
	targetHeight := sourceHeight
	if maxDimension > 0 {
		maxSource := sourceWidth
		if sourceHeight > maxSource {
			maxSource = sourceHeight
		}
		if maxSource > maxDimension {
			scale := float64(maxDimension) / float64(maxSource)
			targetWidth = maxInt(1, int(math.Round(float64(sourceWidth)*scale)))
			targetHeight = maxInt(1, int(math.Round(float64(sourceHeight)*scale)))
		}
	}

	luma := make([]uint8, targetWidth*targetHeight)
	for y := 0; y < targetHeight; y++ {
		sourceY := bounds.Min.Y + int(float64(y)*float64(sourceHeight)/float64(targetHeight))
		if sourceY >= bounds.Max.Y {
			sourceY = bounds.Max.Y - 1
		}
		for x := 0; x < targetWidth; x++ {
			sourceX := bounds.Min.X + int(float64(x)*float64(sourceWidth)/float64(targetWidth))
			if sourceX >= bounds.Max.X {
				sourceX = bounds.Max.X - 1
			}
			r, g, b, _ := img.At(sourceX, sourceY).RGBA()
			// Convert 16-bit RGB channels into 8-bit perceptual luma.
			r8 := float64(r) / 257.0
			g8 := float64(g) / 257.0
			b8 := float64(b) / 257.0
			value := 0.299*r8 + 0.587*g8 + 0.114*b8
			if value < 0 {
				value = 0
			}
			if value > 255 {
				value = 255
			}
			luma[y*targetWidth+x] = uint8(value)
		}
	}
	return luma, targetWidth, targetHeight
}

func sampleImageLumaAndChroma(img image.Image, maxDimension int) ([]uint8, []uint8, int, int) {
	bounds := img.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	targetWidth := sourceWidth
	targetHeight := sourceHeight
	if maxDimension > 0 {
		maxSource := sourceWidth
		if sourceHeight > maxSource {
			maxSource = sourceHeight
		}
		if maxSource > maxDimension {
			scale := float64(maxDimension) / float64(maxSource)
			targetWidth = maxInt(1, int(math.Round(float64(sourceWidth)*scale)))
			targetHeight = maxInt(1, int(math.Round(float64(sourceHeight)*scale)))
		}
	}

	luma := make([]uint8, targetWidth*targetHeight)
	chroma := make([]uint8, targetWidth*targetHeight)
	for y := 0; y < targetHeight; y++ {
		sourceY := bounds.Min.Y + int(float64(y)*float64(sourceHeight)/float64(targetHeight))
		if sourceY >= bounds.Max.Y {
			sourceY = bounds.Max.Y - 1
		}
		for x := 0; x < targetWidth; x++ {
			sourceX := bounds.Min.X + int(float64(x)*float64(sourceWidth)/float64(targetWidth))
			if sourceX >= bounds.Max.X {
				sourceX = bounds.Max.X - 1
			}
			r, g, b, _ := img.At(sourceX, sourceY).RGBA()
			r8 := float64(r) / 257.0
			g8 := float64(g) / 257.0
			b8 := float64(b) / 257.0
			value := 0.299*r8 + 0.587*g8 + 0.114*b8
			if value < 0 {
				value = 0
			}
			if value > 255 {
				value = 255
			}
			luma[y*targetWidth+x] = uint8(value)

			maxRGB := r8
			if g8 > maxRGB {
				maxRGB = g8
			}
			if b8 > maxRGB {
				maxRGB = b8
			}
			minRGB := r8
			if g8 < minRGB {
				minRGB = g8
			}
			if b8 < minRGB {
				minRGB = b8
			}
			diff := maxRGB - minRGB
			if diff < 0 {
				diff = 0
			}
			if diff > 255 {
				diff = 255
			}
			chroma[y*targetWidth+x] = uint8(diff)
		}
	}
	return luma, chroma, targetWidth, targetHeight
}

func adaptiveBrightThreshold(luma []uint8) uint8 {
	if len(luma) == 0 {
		return 185
	}
	sum := 0.0
	sumSquares := 0.0
	for _, value := range luma {
		v := float64(value)
		sum += v
		sumSquares += v * v
	}
	n := float64(len(luma))
	mean := sum / n
	variance := (sumSquares / n) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)
	threshold := mean + (0.2 * stddev)
	if threshold < 165 {
		threshold = 165
	}
	if threshold > 235 {
		threshold = 235
	}
	return uint8(threshold)
}

func adaptivePaperChromaThreshold(chroma []uint8) uint8 {
	if len(chroma) == 0 {
		return 60
	}
	sum := 0.0
	sumSquares := 0.0
	for _, value := range chroma {
		v := float64(value)
		sum += v
		sumSquares += v * v
	}
	n := float64(len(chroma))
	mean := sum / n
	variance := (sumSquares / n) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)
	threshold := mean + (0.45 * stddev)
	if threshold < 36 {
		threshold = 36
	}
	if threshold > 72 {
		threshold = 72
	}
	return uint8(threshold)
}

func largestMaskComponentBounds(mask []bool, width, height int) (int, int, int, int, int, bool) {
	if len(mask) == 0 || width <= 0 || height <= 0 || len(mask) != width*height {
		return 0, 0, 0, 0, 0, false
	}
	visited := make([]bool, len(mask))
	queue := make([]int, 0, 1024)
	bestScore := 0.0
	bestMinX, bestMinY := 0, 0
	bestMaxX, bestMaxY := 0, 0
	bestArea := 0

	for start := 0; start < len(mask); start++ {
		if !mask[start] || visited[start] {
			continue
		}
		queue = queue[:0]
		queue = append(queue, start)
		visited[start] = true

		area := 0
		minX, minY := width-1, height-1
		maxX, maxY := 0, 0
		for head := 0; head < len(queue); head++ {
			index := queue[head]
			x := index % width
			y := index / width
			area++
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx := x + dx
					ny := y + dy
					if nx < 0 || nx >= width || ny < 0 || ny >= height {
						continue
					}
					next := ny*width + nx
					if visited[next] || !mask[next] {
						continue
					}
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}

		bboxWidth := maxX - minX + 1
		bboxHeight := maxY - minY + 1
		if bboxWidth <= 0 || bboxHeight <= 0 {
			continue
		}
		bboxArea := bboxWidth * bboxHeight
		fillRatio := float64(area) / float64(maxInt(1, bboxArea))
		longSide := maxInt(bboxWidth, bboxHeight)
		shortSide := minInt(bboxWidth, bboxHeight)
		aspect := 1.0
		if shortSide > 0 {
			aspect = float64(longSide) / float64(shortSide)
		}
		score := float64(area) * fillRatio * math.Min(aspect, 3.5)
		if score > bestScore {
			bestScore = score
			bestMinX, bestMinY = minX, minY
			bestMaxX, bestMaxY = maxX, maxY
			bestArea = area
		}
	}
	if bestArea == 0 {
		return 0, 0, 0, 0, 0, false
	}
	return bestMinX, bestMinY, bestMaxX, bestMaxY, bestArea, true
}

func copyImageRect(src image.Image, rect image.Rectangle) image.Image {
	rect = rect.Intersect(src.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			dst.Set(x, y, src.At(rect.Min.X+x, rect.Min.Y+y))
		}
	}
	return dst
}

func adaptiveDarkThreshold(luma []uint8) uint8 {
	if len(luma) == 0 {
		return 140
	}
	sum := 0.0
	sumSquares := 0.0
	for _, value := range luma {
		v := float64(value)
		sum += v
		sumSquares += v * v
	}
	n := float64(len(luma))
	mean := sum / n
	variance := (sumSquares / n) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)
	threshold := mean - (0.35 * stddev)
	if threshold < 40 {
		threshold = 40
	}
	if threshold > 220 {
		threshold = 220
	}
	return uint8(threshold)
}

func textOrientationScore(luma []uint8, width, height int, threshold uint8, angle int) float64 {
	rotatedWidth := width
	rotatedHeight := height
	if angle == 90 || angle == 270 {
		rotatedWidth = height
		rotatedHeight = width
	}
	if rotatedWidth <= 0 || rotatedHeight <= 0 {
		return -1e9
	}

	rowCounts := make([]int, rotatedHeight)
	colCounts := make([]int, rotatedWidth)
	totalDark := 0
	for y := 0; y < rotatedHeight; y++ {
		for x := 0; x < rotatedWidth; x++ {
			sourceX, sourceY := mapRotatedPointToSource(x, y, width, height, angle)
			if sourceX < 0 || sourceX >= width || sourceY < 0 || sourceY >= height {
				continue
			}
			if luma[sourceY*width+sourceX] < threshold {
				rowCounts[y]++
				colCounts[x]++
				totalDark++
			}
		}
	}
	if totalDark == 0 {
		return -1e9
	}
	rowCV := coefficientOfVariation(rowCounts)
	colCV := coefficientOfVariation(colCounts)
	density := float64(totalDark) / float64(rotatedWidth*rotatedHeight)
	score := rowCV - colCV
	if density < 0.01 || density > 0.95 {
		score -= 0.25
	}
	return score
}

func coefficientOfVariation(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	sumSquares := 0.0
	for _, value := range values {
		v := float64(value)
		sum += v
		sumSquares += v * v
	}
	n := float64(len(values))
	mean := sum / n
	if mean <= 0 {
		return 0
	}
	variance := (sumSquares / n) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)
	return stddev / mean
}

func mapRotatedPointToSource(x, y, width, height, angle int) (int, int) {
	switch angle {
	case 0:
		return x, y
	case 90:
		return y, height - 1 - x
	case 180:
		return width - 1 - x, height - 1 - y
	case 270:
		return width - 1 - y, x
	default:
		return x, y
	}
}

func rotateImageQuarterTurns(src image.Image, angle int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	switch angle {
	case 0:
		return src
	case 90:
		dst := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < width; y++ {
			for x := 0; x < height; x++ {
				sourceX := bounds.Min.X + y
				sourceY := bounds.Min.Y + (height - 1 - x)
				dst.Set(x, y, src.At(sourceX, sourceY))
			}
		}
		return dst
	case 180:
		dst := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				sourceX := bounds.Min.X + (width - 1 - x)
				sourceY := bounds.Min.Y + (height - 1 - y)
				dst.Set(x, y, src.At(sourceX, sourceY))
			}
		}
		return dst
	case 270:
		dst := image.NewRGBA(image.Rect(0, 0, height, width))
		for y := 0; y < width; y++ {
			for x := 0; x < height; x++ {
				sourceX := bounds.Min.X + (width - 1 - y)
				sourceY := bounds.Min.Y + x
				dst.Set(x, y, src.At(sourceX, sourceY))
			}
		}
		return dst
	default:
		return src
	}
}

func collectFallbackReceiptLines(result *ReceiptParseResult) []string {
	lines := make([]string, 0, 48)
	seen := map[string]struct{}{}
	add := func(text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		for _, segment := range fallbackLineSplitPattern.Split(text, -1) {
			line := strings.Join(strings.Fields(strings.TrimSpace(segment)), " ")
			if line == "" {
				continue
			}
			key := strings.ToLower(line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			lines = append(lines, line)
		}
	}
	for _, item := range result.Items {
		add(item.Name)
		add(ptrString(item.RawText))
	}
	for _, line := range result.UnparsedLines {
		add(line)
	}
	return lines
}

func parseFallbackItemsFromLines(lines []string) []ReceiptItem {
	items := make([]ReceiptItem, 0, len(lines))
	for _, line := range lines {
		amountToken, amountCents, ok := rightmostMoneyToken(line)
		if !ok || amountCents <= 0 {
			continue
		}
		if !strings.Contains(amountToken, ".") && !strings.ContainsAny(line, "$¥€£₩₹") {
			continue
		}
		namePart := strings.TrimSpace(line)
		if idx := strings.LastIndex(namePart, amountToken); idx >= 0 {
			namePart = strings.TrimSpace(namePart[:idx])
		}
		name := normalizeFallbackItemName(namePart)
		if fallbackShouldSkipLine(strings.ToLower(name)) {
			continue
		}
		qty := 1.0
		if match := fallbackLeadingQtyPattern.FindStringSubmatch(name); len(match) == 3 {
			if parsedQty, err := strconv.Atoi(match[1]); err == nil && parsedQty >= 2 && parsedQty <= 24 {
				tail := strings.TrimSpace(match[2])
				if hasAnyLetter(tail) {
					name = tail
					qty = float64(parsedQty)
				}
			}
		}
		if strings.EqualFold(name, "guest") {
			continue
		}
		unitCents := amountCents
		if qty > 1 {
			unitCents = int(math.Round(float64(amountCents) / qty))
			if unitCents <= 0 {
				unitCents = amountCents
				qty = 1
			}
		}
		raw := line
		qtyValue := qty
		unitValue := unitCents
		lineValue := amountCents
		items = append(items, ReceiptItem{
			Name:           name,
			Quantity:       &qtyValue,
			UnitPriceCents: &unitValue,
			LinePriceCents: &lineValue,
			RawText:        &raw,
		})
	}
	return items
}

func rightmostMoneyToken(line string) (string, int, bool) {
	matches := moneyTokenPattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return "", 0, false
	}
	last := matches[len(matches)-1]
	token := line[last[0]:last[1]]
	cents, ok := parseMoneyTokenToCents(token)
	if !ok {
		return "", 0, false
	}
	return token, cents, true
}

func normalizeFallbackItemName(name string) string {
	out := strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	out = fallbackGuestNumberPrefixPattern.ReplaceAllString(out, "")
	out = fallbackGuestWordPrefixPattern.ReplaceAllString(out, "")
	out = fallbackSeatPrefixPattern.ReplaceAllString(out, "")
	out = fallbackLeadingCodePattern.ReplaceAllString(out, "")
	return normalizeReceiptLabel(out)
}

func normalizeReceiptLabel(name string) string {
	out := strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	if out == "" {
		return ""
	}
	for {
		trimmed := strings.TrimSpace(strings.Trim(out, "-:;,.|"))
		next := strings.TrimSpace(trailingCurrencySymbolPattern.ReplaceAllString(trimmed, ""))
		if next == out {
			return next
		}
		out = next
	}
}

func fallbackShouldSkipLine(nameLower string) bool {
	if nameLower == "" || !hasAnyLetter(nameLower) {
		return true
	}
	for _, keyword := range fallbackNonItemKeywords {
		if strings.Contains(nameLower, keyword) {
			return true
		}
	}
	return false
}

func looksLikeBeverageLine(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	for _, token := range beverageKeywords {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func hasAnyLetter(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func normalizeReceiptItemPricing(item *ReceiptItem) {
	if item == nil {
		return
	}
	qty := 1.0
	if item.Quantity != nil && *item.Quantity > 0 {
		qty = *item.Quantity
	}

	observedMoney := extractMoneyCentsFromText(strings.TrimSpace(strings.Join([]string{
		ptrString(item.RawText),
		item.Name,
	}, " ")))
	bestObservedLine := maxMoneyCandidate(observedMoney)

	lineCents := 0
	if item.LinePriceCents != nil && *item.LinePriceCents > 0 {
		lineCents = *item.LinePriceCents
	}
	unitCents := 0
	if item.UnitPriceCents != nil && *item.UnitPriceCents > 0 {
		unitCents = *item.UnitPriceCents
	}
	if bestObservedLine > 0 {
		if lineCents <= 0 {
			lineCents = bestObservedLine
		} else {
			// Correct obvious magnitude mistakes like 900 vs 93000 when raw line carries "930.00".
			if bestObservedLine >= lineCents*10 {
				lineCents = bestObservedLine
			}
			// If model likely wrote unit as line total, prefer observed amount that matches qty * unit.
			if unitCents > 0 && qty > 1 {
				expectedFromUnit := int(math.Round(float64(unitCents) * qty))
				bestMatchesUnit := absInt(bestObservedLine-expectedFromUnit) <= maxInt(200, expectedFromUnit/25)
				lineMismatch := absInt(lineCents-expectedFromUnit) > maxInt(200, expectedFromUnit/25)
				if bestMatchesUnit && lineMismatch {
					lineCents = bestObservedLine
				}
			}
		}
	}
	if lineCents > 0 {
		item.LinePriceCents = intPtr(lineCents)
		unitFromLine := int(math.Round(float64(lineCents) / qty))
		if unitFromLine <= 0 {
			unitFromLine = lineCents
		}
		if unitCents <= 0 {
			item.UnitPriceCents = intPtr(unitFromLine)
			return
		}
		expectedLine := int(math.Round(float64(unitCents) * qty))
		if absInt(expectedLine-lineCents) > maxInt(200, lineCents/20) {
			item.UnitPriceCents = intPtr(unitFromLine)
		}
	}
}

func extractMoneyCentsFromText(text string) []int {
	matches := moneyTokenPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]int, 0, len(matches))
	for _, token := range matches {
		cents, ok := parseMoneyTokenToCents(token)
		if ok && cents > 0 {
			out = append(out, cents)
		}
	}
	return out
}

func singleUniqueMoneyCandidate(candidates []int) (int, bool) {
	if len(candidates) == 0 {
		return 0, false
	}
	seen := make(map[int]struct{}, len(candidates))
	unique := 0
	for _, candidate := range candidates {
		if candidate <= 0 {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		unique = candidate
		if len(seen) > 1 {
			return 0, false
		}
	}
	if len(seen) != 1 {
		return 0, false
	}
	return unique, true
}

func parseMoneyTokenToCents(token string) (int, bool) {
	clean := strings.ReplaceAll(strings.TrimSpace(token), ",", "")
	if clean == "" {
		return 0, false
	}
	if !strings.Contains(clean, ".") {
		whole, err := strconv.Atoi(clean)
		if err != nil || whole <= 0 {
			return 0, false
		}
		// Integer tokens are ambiguous across currencies/locales (e.g., JPY has no decimal minor units).
		// Keep integer magnitude as-is and rely on explicit decimal tokens for cents correction.
		return whole, true
	}
	value, err := strconv.ParseFloat(clean, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return int(math.Round(value * 100)), true
}

func maxMoneyCandidate(candidates []int) int {
	best := 0
	for _, candidate := range candidates {
		if candidate > best {
			best = candidate
		}
	}
	return best
}

func repairItemLinePricesAgainstSubtotal(result *ReceiptParseResult) {
	if result == nil || result.SubtotalCents == nil || *result.SubtotalCents <= 0 || len(result.Items) == 0 {
		return
	}
	targetSubtotal := *result.SubtotalCents
	currentSubtotal := maxInt(0, receiptItemsNetSubtotal(result.Items)-receiptBillDiscountCents(result))
	if absInt(currentSubtotal-targetSubtotal) <= 1 {
		return
	}
	for i := range result.Items {
		item := &result.Items[i]
		rawText := strings.TrimSpace(ptrString(item.RawText))
		if rawText == "" {
			continue
		}
		rawLineCents, ok := singleUniqueMoneyCandidate(extractMoneyCentsFromText(rawText))
		if !ok || rawLineCents <= 0 {
			continue
		}
		currentLineCents := receiptItemLineCents(*item)
		if currentLineCents > 0 && absInt(rawLineCents-currentLineCents) <= maxInt(100, currentLineCents/20) {
			continue
		}
		currentItemNet := receiptItemNetCents(*item)
		qty := receiptItemQuantity(*item)
		candidateItemNet := rawLineCents
		if item.DiscountCents != nil && *item.DiscountCents > 0 {
			candidateItemNet -= int(math.Round(float64(*item.DiscountCents) * qty))
			if candidateItemNet < 0 {
				candidateItemNet = 0
			}
		}
		candidateSubtotal := currentSubtotal - currentItemNet + candidateItemNet
		if absInt(candidateSubtotal-targetSubtotal) >= absInt(currentSubtotal-targetSubtotal) {
			continue
		}
		item.LinePriceCents = intPtr(rawLineCents)
		unitFromLine := int(math.Round(float64(rawLineCents) / qty))
		if unitFromLine <= 0 {
			unitFromLine = rawLineCents
		}
		item.UnitPriceCents = intPtr(unitFromLine)
		currentSubtotal = candidateSubtotal
		if absInt(currentSubtotal-targetSubtotal) <= 1 {
			return
		}
	}
}

func normalizeSubtotalFromTotalAndTax(result *ReceiptParseResult) {
	if result == nil || result.SubtotalCents == nil || result.TotalCents == nil {
		return
	}
	subtotal := *result.SubtotalCents
	total := *result.TotalCents
	if subtotal <= 0 || total <= 0 {
		return
	}
	tax := 0
	if result.TaxCents != nil && *result.TaxCents > 0 {
		tax = *result.TaxCents
	}
	tip := 0
	if result.TipCents != nil && *result.TipCents > 0 {
		tip = *result.TipCents
	}
	derivedSubtotal := total - tax - tip
	if derivedSubtotal <= 0 {
		return
	}
	lo := subtotal
	hi := derivedSubtotal
	if lo > hi {
		lo, hi = hi, lo
	}
	// Only repair obvious order-of-magnitude OCR misses (e.g., 11930 vs 123480).
	if lo > 0 && float64(hi)/float64(lo) >= 8.0 {
		result.SubtotalCents = intPtr(derivedSubtotal)
	}
}

func ptrString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func intPtr(v int) *int {
	n := v
	return &n
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	roomID := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/ws/")))
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

func normalizeVenmoUsername(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimLeft(trimmed, "@")
	if trimmed == "" {
		return ""
	}
	return strings.Join(strings.Fields(trimmed), "")
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
