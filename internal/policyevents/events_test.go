package policyevents

import (
	"bytes"
	"testing"
)

func TestBoundedRoundTrip(t *testing.T) {
	var out bytes.Buffer
	s := New(2, &out)
	for _, code := range []string{"a", "b", "c"} {
		s.Add(Event{Code: code})
	}
	got := s.Since(0)
	if len(got) != 2 || got[0].Sequence != 2 {
		t.Fatal(got)
	}
	got[0].Code = "mutated"
	if s.Since(1)[0].Code != "b" {
		t.Fatal("mutable snapshot")
	}
	n := 0
	if err := Read(&out, func(e Event) error { n++; return nil }); err != nil || n != 3 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestWriterLimitPreservesRecentInMemoryEvents(t *testing.T) {
	var out bytes.Buffer
	s := New(2, &out)
	s.writeLimit = 2
	for i := 0; i < 10; i++ {
		s.Add(Event{Code: "domain_denied"})
	}
	var codes []string
	if err := Read(&out, func(e Event) error { codes = append(codes, e.Code); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(codes) != 3 || codes[2] != "event_log_limit" {
		t.Fatal(codes)
	}
	events := s.Since(0)
	if len(events) != 2 || events[1].Sequence != 10 || events[1].Code != "domain_denied" {
		t.Fatal(events)
	}
}
