package main

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestLimitedLogDrainsAfterCapAcrossStreams(t *testing.T) {
	var file bytes.Buffer
	writer := &limitedLog{file: &file, remaining: 8}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, err := writer.Write([]byte(strings.Repeat("x", 32))); err != nil || n != 32 {
				t.Errorf("%d %v", n, err)
			}
		}()
	}
	wg.Wait()
	if file.Len() != 8 || writer.remaining != 0 {
		t.Fatalf("retained %d", file.Len())
	}
}
