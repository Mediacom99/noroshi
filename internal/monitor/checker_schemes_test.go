package monitor

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestCheckerTCPUp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	checker := NewChecker(2 * time.Second)
	res := checker.Check(context.Background(), "tcp://"+ln.Addr().String(), CheckOptions{})
	if !res.Up {
		t.Errorf("Up = false, want true (reason: %s, err: %v)", res.Reason, res.Err)
	}
	if res.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 for TCP checks", res.StatusCode)
	}
	if res.Latency <= 0 {
		t.Errorf("Latency = %v, want > 0", res.Latency)
	}
}

func TestCheckerTCPRefused(t *testing.T) {
	checker := NewChecker(1 * time.Second)
	res := checker.Check(context.Background(), "tcp://127.0.0.1:1", CheckOptions{})
	if res.Up {
		t.Error("Up = true, want false for refused connection")
	}
	if res.Err == nil {
		t.Error("expected error for refused connection")
	}
	if res.Reason != "connection error" {
		t.Errorf("Reason = %q, want %q", res.Reason, "connection error")
	}
}

func TestCheckerDNS(t *testing.T) {
	checker := NewChecker(5 * time.Second)

	res := checker.Check(context.Background(), "dns://localhost", CheckOptions{})
	if !res.Up {
		t.Errorf("Up = false, want true for dns://localhost (reason: %s, err: %v)", res.Reason, res.Err)
	}

	res = checker.Check(context.Background(), "dns://nonexistent.invalid", CheckOptions{})
	if res.Up {
		t.Error("Up = true, want false for unresolvable name")
	}
	if res.Reason != "dns lookup failed" {
		t.Errorf("Reason = %q, want %q", res.Reason, "dns lookup failed")
	}
}

func TestCheckerPing(t *testing.T) {
	checker := NewChecker(2 * time.Second)
	res := checker.Check(context.Background(), "ping://127.0.0.1", CheckOptions{})
	if res.Reason == "ping not permitted" {
		t.Skipf("unprivileged ping not available in this environment: %v", res.Err)
	}
	if !res.Up {
		t.Errorf("Up = false, want true for ping://127.0.0.1 (reason: %s, err: %v)", res.Reason, res.Err)
	}
}

func TestCheckerPingUnresolvable(t *testing.T) {
	checker := NewChecker(2 * time.Second)
	res := checker.Check(context.Background(), "ping://nonexistent.invalid", CheckOptions{})
	if res.Up {
		t.Error("Up = true, want false for unresolvable host")
	}
	if res.Reason != "dns lookup failed" {
		t.Errorf("Reason = %q, want %q", res.Reason, "dns lookup failed")
	}
}
