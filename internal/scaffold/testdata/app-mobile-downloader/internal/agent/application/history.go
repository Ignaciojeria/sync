package application

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type ConversationHistory struct {
	Items      []ConversationItem `json:"items"`
	LastSeq    uint64             `json:"lastSeq"`
	NextBefore uint64             `json:"nextBefore,omitempty"`
	HasMore    bool               `json:"hasMore"`
}

type ConversationItem struct {
	Seq      uint64          `json:"seq"`
	Kind     string          `json:"kind"`
	Text     string          `json:"text,omitempty"`
	ToolName string          `json:"toolName,omitempty"`
	Args     json.RawMessage `json:"args,omitempty"`
}

type RuntimeEventRecord struct {
	Offset  uint64
	Kind    string
	Payload json.RawMessage
}

type RuntimeEventsHistorySource interface {
	ListSession(ctx context.Context, sessionID string) ([]RuntimeEventRecord, error)
}

var runtimeEventsHistorySource RuntimeEventsHistorySource

func SetRuntimeEventsHistorySource(source RuntimeEventsHistorySource) {
	runtimeEventsHistorySource = source
}

var transcriptState = struct {
	sync.Mutex
	assistant map[string]ConversationItem
}{assistant: map[string]ConversationItem{}}

func LoadConversationHistory(sessionID string, before uint64, limit int) (ConversationHistory, error) {
	return LoadConversationHistoryCtx(context.Background(), sessionID, before, limit)
}

func LoadConversationHistoryCtx(ctx context.Context, sessionID string, before uint64, limit int) (ConversationHistory, error) {
	if strings.TrimSpace(sessionID) == "" {
		return ConversationHistory{}, nil
	}
	if limit <= 0 {
		limit = 30
	}
	var (
		items []ConversationItem
		err   error
	)
	if runtimeEventsHistorySource != nil {
		var rows []RuntimeEventRecord
		rows, err = runtimeEventsHistorySource.ListSession(ctx, sessionID)
		if err == nil && len(rows) > 0 {
			items = rebuildConversationFromRuntimeRecords(rows)
		}
	}
	if len(items) == 0 {
		items, err = readConversationTranscript(sessionID)
		if err != nil {
			return ConversationHistory{}, err
		}
	}
	if before > 0 {
		filtered := items[:0]
		for _, item := range items {
			if item.Seq < before {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if len(items) == 0 {
		return ConversationHistory{}, nil
	}
	out := ConversationHistory{LastSeq: items[len(items)-1].Seq}
	if len(items) > limit {
		out.HasMore = true
		out.NextBefore = items[len(items)-limit].Seq
		items = items[len(items)-limit:]
	}
	out.Items = items
	out.LastSeq = items[len(items)-1].Seq
	if before == 0 && limit == 1 && len(out.Items) == 1 {
		out.Items[0].Text = previewText(out.Items[0].Text, 1200)
	}
	return out, nil
}

func MaterializeUserPrompt(sessionID, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	_ = appendTranscriptItem(sessionID, ConversationItem{Kind: "user", Text: text})
}

func MaterializeEvent(sessionID string, seq uint64, event Event) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	switch event.Type {
	case "message_start":
		if text, ok := extractAssistantStopError(event.Payload); ok {
			_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "error", Text: text})
			return
		}
		transcriptState.Lock()
		transcriptState.assistant[sessionID] = ConversationItem{Seq: seq, Kind: "assistant"}
		transcriptState.Unlock()
	case "message_update":
		var payload struct {
			AssistantMessageEvent struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
			} `json:"assistantMessageEvent"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.AssistantMessageEvent.Type != "text_delta" {
			return
		}
		transcriptState.Lock()
		item := transcriptState.assistant[sessionID]
		item.Kind = "assistant"
		item.Seq = seq
		item.Text += payload.AssistantMessageEvent.Delta
		transcriptState.assistant[sessionID] = item
		transcriptState.Unlock()
	case "message_end":
		transcriptState.Lock()
		item, ok := transcriptState.assistant[sessionID]
		delete(transcriptState.assistant, sessionID)
		transcriptState.Unlock()
		if !ok || strings.TrimSpace(item.Text) == "" {
			return
		}
		if item.Seq == 0 {
			item.Seq = seq
		}
		_ = appendTranscriptItem(sessionID, item)
	case "tool_execution_start":
		var payload struct {
			ToolName string          `json:"toolName"`
			Type     string          `json:"type"`
			Args     json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		name := strings.TrimSpace(payload.ToolName)
		if name == "" {
			name = strings.TrimSpace(payload.Type)
		}
		_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "tool", ToolName: name, Args: payload.Args})
	case "runtime_error", "stderr", "runtime_exit":
		var payload struct {
			Message json.RawMessage `json:"message"`
			Reason  string          `json:"reason"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		text := extractMessageText(payload.Message)
		if text == "" {
			text = strings.TrimSpace(payload.Reason)
		}
		if text == "" {
			text = event.Type
		}
		_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "error", Text: text})
	}
}

func readConversationTranscript(sessionID string) ([]ConversationItem, error) {
	path := transcriptPath(sessionID)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		items, backfillErr := buildTranscriptFromLegacyJournal(sessionID)
		if backfillErr != nil {
			return nil, backfillErr
		}
		if len(items) > 0 {
			_ = rewriteTranscript(sessionID, items)
		}
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []ConversationItem
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var item ConversationItem
		if err := json.Unmarshal(sc.Bytes(), &item); err == nil && strings.TrimSpace(item.Kind) != "" {
			out = append(out, item)
		}
	}
	return out, sc.Err()
}

func appendTranscriptItem(sessionID string, item ConversationItem) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(item.Kind) == "" {
		return nil
	}
	transcriptState.Lock()
	defer transcriptState.Unlock()
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		return err
	}
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(transcriptPath(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(body, '\n'))
	return err
}

func rewriteTranscript(sessionID string, items []ConversationItem) error {
	transcriptState.Lock()
	defer transcriptState.Unlock()
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		return err
	}
	f, err := os.Create(transcriptPath(sessionID))
	if err != nil {
		return err
	}
	defer f.Close()
	for _, item := range items {
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(body, '\n')); err != nil {
			return err
		}
	}
	return nil
}

type historyJournalEntry struct {
	Seq     uint64          `json:"seq"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func buildTranscriptFromLegacyJournal(sessionID string) ([]ConversationItem, error) {
	entries, err := readSessionJournal(sessionID)
	if err != nil {
		return nil, err
	}
	return rebuildConversation(entries), nil
}

func readSessionJournal(sessionID string) ([]historyJournalEntry, error) {
	path := filepath.Join(eventsJournalDir(), sanitizeSessionID(sessionID)+".jsonl")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []historyJournalEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e historyJournalEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

func rebuildConversation(entries []historyJournalEntry) []ConversationItem {
	records := make([]RuntimeEventRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, RuntimeEventRecord{Offset: entry.Seq, Kind: entry.Kind, Payload: entry.Payload})
	}
	return rebuildConversationFromRuntimeRecords(records)
}

func rebuildConversationFromRuntimeRecords(records []RuntimeEventRecord) []ConversationItem {
	items := make([]ConversationItem, 0, len(records))
	current := ConversationItem{Kind: "assistant"}
	flushAssistant := func() {
		if strings.TrimSpace(current.Text) == "" {
			current = ConversationItem{Kind: "assistant"}
			return
		}
		items = append(items, current)
		current = ConversationItem{Kind: "assistant"}
	}
	for _, record := range records {
		var event Event
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			continue
		}
		switch event.Type {
		case "user_prompt":
			flushAssistant()
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			text := strings.TrimSpace(payload.Text)
			if text == "" {
				continue
			}
			items = append(items, ConversationItem{Seq: record.Offset, Kind: "user", Text: text})
		case "message_start":
			flushAssistant()
			if text, ok := extractAssistantStopError(event.Payload); ok {
				items = append(items, ConversationItem{Seq: record.Offset, Kind: "error", Text: text})
			}
		case "message_update":
			var payload struct {
				AssistantMessageEvent struct {
					Type  string `json:"type"`
					Delta string `json:"delta"`
				} `json:"assistantMessageEvent"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			if payload.AssistantMessageEvent.Type != "text_delta" {
				continue
			}
			current.Kind = "assistant"
			current.Seq = record.Offset
			current.Text += payload.AssistantMessageEvent.Delta
		case "message_end":
			flushAssistant()
		case "tool_execution_start":
			flushAssistant()
			var payload struct {
				ToolName string          `json:"toolName"`
				Type     string          `json:"type"`
				Args     json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			name := strings.TrimSpace(payload.ToolName)
			if name == "" {
				name = strings.TrimSpace(payload.Type)
			}
			items = append(items, ConversationItem{Seq: record.Offset, Kind: "tool", ToolName: name, Args: payload.Args})
		case "runtime_error", "stderr", "runtime_exit":
			flushAssistant()
			var payload struct {
				Message json.RawMessage `json:"message"`
				Reason  string          `json:"reason"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			text := extractMessageText(payload.Message)
			if text == "" {
				text = strings.TrimSpace(payload.Reason)
			}
			if text == "" {
				text = event.Type
			}
			items = append(items, ConversationItem{Seq: record.Offset, Kind: "error", Text: text})
		}
	}
	flushAssistant()
	return items
}

func ParseHistoryBefore(raw string) uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func ParseHistoryLimit(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 50 {
		n = 50
	}
	return n
}

func eventsJournalDir() string {
	dir := strings.TrimSpace(os.Getenv("AGENT_EVENTS_DIR"))
	if dir == "" {
		dir = "tmp/agent-events"
	}
	return dir
}

func transcriptDir() string {
	dir := strings.TrimSpace(os.Getenv("AGENT_TRANSCRIPTS_DIR"))
	if dir == "" {
		dir = "tmp/agent-transcripts"
	}
	return dir
}

func transcriptPath(sessionID string) string {
	return filepath.Join(transcriptDir(), sanitizeSessionID(sessionID)+".jsonl")
}

func sanitizeSessionID(sessionID string) string {
	safe := strings.ReplaceAll(sessionID, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	safe = strings.ReplaceAll(safe, string(filepath.Separator), "_")
	return safe
}

func previewText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "\n\n…"
}

func extractMessageText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return trimmed
}

func extractAssistantStopError(raw json.RawMessage) (string, bool) {
	var payload struct {
		Message struct {
			Role         string `json:"role"`
			StopReason   string `json:"stopReason"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	if strings.TrimSpace(payload.Message.Role) != "assistant" {
		return "", false
	}
	text := strings.TrimSpace(payload.Message.ErrorMessage)
	if text == "" {
		return "", false
	}
	if normalized := normalizeAssistantErrorText(text, payload.Message.StopReason); normalized != "" {
		return normalized, true
	}
	return text, true
}

func normalizeAssistantErrorText(text, stopReason string) string {
	combined := strings.ToLower(strings.TrimSpace(stopReason + " " + text))
	if strings.Contains(combined, "insufficient_credits") ||
		strings.Contains(combined, "payment required") ||
		strings.Contains(combined, "402") ||
		strings.Contains(combined, "créditos insuficientes") {
		return "Créditos insuficientes en el proveedor/modelo configurado."
	}
	return strings.TrimSpace(text)
}
