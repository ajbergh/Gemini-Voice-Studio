// Copyright 2025 ajbergh
// SPDX-License-Identifier: Apache-2.0

// Package handler - ws_progress.go manages real-time job progress WebSockets
// with bounded per-client queues and persistent job-state mirroring.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ajbergh/gemini-voice-gen-tts/backend/internal/store"
	"nhooyr.io/websocket"
)

const progressClientQueueSize = 64

// ProgressEvent represents a real-time progress update sent to clients.
type ProgressEvent struct {
	JobID          string `json:"job_id,omitempty"`
	Type           string `json:"type"`
	Status         string `json:"status"`
	Message        string `json:"message,omitempty"`
	Percent        int    `json:"percent"`
	ItemID         string `json:"item_id,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	SegmentID      string `json:"segment_id,omitempty"`
	CompletedItems int    `json:"completed_items,omitempty"`
	TotalItems     int    `json:"total_items,omitempty"`
	FailedItems    int    `json:"failed_items,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
}

type progressClient struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
	send   chan []byte
	once   sync.Once
}

func (client *progressClient) close(status websocket.StatusCode, reason string) {
	client.once.Do(func() {
		client.cancel()
		_ = client.conn.Close(status, reason)
	})
}

// ProgressHub manages WebSocket connections and broadcasts progress events.
type ProgressHub struct {
	mu      sync.RWMutex
	clients map[*progressClient]struct{}
	Store   *store.Store
}

// NewProgressHub creates a new ProgressHub.
func NewProgressHub(st *store.Store) *ProgressHub {
	return &ProgressHub{clients: make(map[*progressClient]struct{}), Store: st}
}

// HandleWS upgrades, registers, and reads until the client disconnects.
func (h *ProgressHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost", "127.0.0.1", "::1", "[::1]"},
	})
	if err != nil {
		slog.Warn("websocket accept failed", "error", err)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	client := &progressClient{
		conn: connection, ctx: ctx, cancel: cancel,
		send: make(chan []byte, progressClientQueueSize),
	}
	count := h.addClient(client)
	slog.Debug("websocket client connected", "clients", count)
	go h.writeClient(client)

	welcome, _ := json.Marshal(ProgressEvent{Type: "system", Status: "connected", Message: "Progress updates active"})
	select {
	case client.send <- welcome:
	case <-client.ctx.Done():
	}

	for {
		if _, _, err := connection.Read(ctx); err != nil {
			break
		}
	}
	h.removeClient(client, websocket.StatusNormalClosure, "")
}

func (h *ProgressHub) addClient(client *progressClient) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
	return len(h.clients)
}

func (h *ProgressHub) removeClient(client *progressClient, status websocket.StatusCode, reason string) {
	h.mu.Lock()
	_, existed := h.clients[client]
	if existed {
		delete(h.clients, client)
	}
	count := len(h.clients)
	h.mu.Unlock()
	if existed {
		client.close(status, reason)
		slog.Debug("websocket client disconnected", "clients", count, "reason", reason)
	}
}

func (h *ProgressHub) writeClient(client *progressClient) {
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	for {
		select {
		case <-client.ctx.Done():
			return
		case data := <-client.send:
			writeCtx, cancel := context.WithTimeout(client.ctx, 5*time.Second)
			err := client.conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				h.removeClient(client, websocket.StatusGoingAway, "write failed")
				return
			}
		case <-pingTicker.C:
			pingCtx, cancel := context.WithTimeout(client.ctx, 5*time.Second)
			err := client.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				h.removeClient(client, websocket.StatusGoingAway, "ping failed")
				return
			}
		}
	}
}

// Broadcast persists and enqueues an event without allowing one slow browser to
// block render workers or other connected clients.
func (h *ProgressHub) Broadcast(event ProgressEvent) {
	h.persistEvent(event)
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := make([]*progressClient, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	for _, client := range clients {
		select {
		case client.send <- data:
		case <-client.ctx.Done():
		default:
			h.removeClient(client, websocket.StatusPolicyViolation, "progress queue overflow")
		}
	}
}

// EmitProgress is a convenience method for emitting a job event.
func (h *ProgressHub) EmitProgress(jobID, jobType, status, message string, percent int) {
	h.Broadcast(ProgressEvent{JobID: jobID, Type: jobType, Status: status, Message: message, Percent: percent})
}

func (h *ProgressHub) persistEvent(event ProgressEvent) {
	if h.Store == nil || event.JobID == "" || event.Type == "system" {
		return
	}
	if err := h.Store.UpsertJobProgress(store.JobProgressUpdate{
		ID: event.JobID, Type: event.Type, Status: event.Status, Message: event.Message,
		Percent: event.Percent, ProjectID: event.ProjectID, SegmentID: event.SegmentID,
		TotalItems: event.TotalItems, CompletedItems: event.CompletedItems,
		FailedItems: event.FailedItems, ErrorCode: event.ErrorCode,
	}); err != nil {
		slog.Warn("failed to persist progress event", "job_id", event.JobID, "error", err)
	}
}
