package session_test

import (
	"testing"
	"time"

	"xata/services/gateway/session"
)

func TestNoopSession_DoesNotHang(t *testing.T) {
	done := make(chan bool)
	go func() {
		session.NoopSession()
		done <- true
	}()

	select {
	case <-done:
		// Test passed: NoopSession returned without hanging
	case <-time.After(1 * time.Second):
		t.Fatal("NoopSession hung for more than 1 second")
	}
}
