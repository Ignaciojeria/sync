package handlers

import (
	"testing"
)

func openJournalForTest(t *testing.T) *eventsJournal {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGENT_EVENTS_DIR", dir)
	journalOpenMu.Lock()
	journalGlobal = nil
	journalOpenMu.Unlock()
	j, err := openEventsJournal()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return j
}

func TestJournal_AppendAndReplay(t *testing.T) {
	j := openJournalForTest(t)
	for i := 1; i <= 5; i++ {
		if _, err := j.append("sess", "pi", map[string]int{"i": i}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	dst, err := j.replay("sess", 0, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(dst) != 5 {
		t.Fatalf("replay desde 0: got %d, want 5", len(dst))
	}
	for i, e := range dst {
		if e.Seq != uint64(i+1) {
			t.Fatalf("entry %d: seq=%d, want %d", i, e.Seq, i+1)
		}
	}
	dst, _ = j.replay("sess", 3, nil)
	if len(dst) != 2 {
		t.Fatalf("replay desde 3: got %d, want 2", len(dst))
	}
	if dst[0].Seq != 4 {
		t.Fatalf("primer seq después de 3: %d, want 4", dst[0].Seq)
	}
}

func TestJournal_EmptySession(t *testing.T) {
	j := openJournalForTest(t)
	dst, err := j.replay("nonexistent", 0, nil)
	if err != nil {
		t.Fatalf("replay empty: %v", err)
	}
	if len(dst) != 0 {
		t.Fatalf("got %d entries, want 0", len(dst))
	}
	last, err := j.lastSeq("nonexistent")
	if err != nil {
		t.Fatalf("lastSeq empty: %v", err)
	}
	if last != 0 {
		t.Fatalf("lastSeq empty: %d, want 0", last)
	}
}

func TestJournal_AppendOnce_DedupesSameEvent(t *testing.T) {
	j := openJournalForTest(t)
	payload := map[string]any{"type": "message_end", "createdAt": "2026-07-08T01:47:16Z"}
	seq1, appended1, err := j.appendOnce("sess", "pi", payload)
	if err != nil {
		t.Fatalf("appendOnce #1: %v", err)
	}
	seq2, appended2, err := j.appendOnce("sess", "pi", payload)
	if err != nil {
		t.Fatalf("appendOnce #2: %v", err)
	}
	if !appended1 || appended2 {
		t.Fatalf("appended flags = (%v,%v), want (true,false)", appended1, appended2)
	}
	if seq1 != 1 || seq2 != 1 {
		t.Fatalf("seqs = (%d,%d), want (1,1)", seq1, seq2)
	}
	dst, err := j.replay("sess", 0, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(dst) != 1 {
		t.Fatalf("replay len = %d, want 1", len(dst))
	}
}

func TestParseSince(t *testing.T) {
	cases := map[string]uint64{
		"":      0,
		"  ":    0,
		"abc":   0,
		"0":     0,
		"42":    42,
		"99999": 99999,
		"  -3 ": 0,
	}
	for in, want := range cases {
		if got := parseSince(in); got != want {
			t.Errorf("parseSince(%q) = %d, want %d", in, got, want)
		}
	}
}
