// Package policyevents records bounded, credential-free policy diagnostics.
package policyevents

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Type        string    `json:"type"`
	Sequence    uint64    `json:"sequence"`
	Time        time.Time `json:"time"`
	PolicyID    string    `json:"policy_id,omitempty"`
	Worker      string    `json:"worker,omitempty"`
	RunID       string    `json:"run_id,omitempty"`
	Source      string    `json:"source,omitempty"`
	Destination string    `json:"destination,omitempty"`
	Code        string    `json:"code"`
}

const MaxWrittenEvents = 100000

type Store struct {
	written    int
	writeLimit int
	mu         sync.Mutex
	capacity   int
	sequence   uint64
	events     []Event
	writer     io.Writer
}

func New(capacity int, writer io.Writer) *Store {
	if capacity < 1 {
		capacity = 512
	}
	return &Store{capacity: capacity, writer: writer, writeLimit: MaxWrittenEvents}
}
func (s *Store) Add(e Event) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	e.Type = "cxd.policy"
	e.Sequence = s.sequence
	e.Time = time.Now().UTC()
	if len(s.events) == s.capacity {
		copy(s.events, s.events[1:])
		s.events[len(s.events)-1] = e
	} else {
		s.events = append(s.events, e)
	}
	if s.writer != nil && s.written <= s.writeLimit {
		if s.written == s.writeLimit {
			e = Event{Type: "cxd.policy", Sequence: e.Sequence, Time: e.Time, Code: "event_log_limit"}
		}
		_ = json.NewEncoder(s.writer).Encode(e)
		s.written++
	}
}
func (s *Store) Since(sequence uint64) []Event {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Event
	for _, e := range s.events {
		if e.Sequence > sequence {
			out = append(out, e)
		}
	}
	return out
}

// Read ignores ordinary console lines. The bounded scanner rejects oversized
// records, rather than allocating arbitrarily on untrusted process output.
func Read(r io.Reader, visit func(Event) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil || e.Type != "cxd.policy" {
			continue
		}
		if err := visit(e); err != nil {
			return err
		}
	}
	return scanner.Err()
}
