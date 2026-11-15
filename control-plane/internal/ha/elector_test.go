package ha

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestElectionConfigNormalizeRenewLessThanTTL(t *testing.T) {
	cfg := ElectionConfig{}.Normalize()
	if cfg.TTL != 15*time.Second {
		t.Fatalf("ttl=%v", cfg.TTL)
	}
	if cfg.RenewEvery >= cfg.TTL {
		t.Fatalf("renew %v must be < ttl %v", cfg.RenewEvery, cfg.TTL)
	}
	if cfg.Name != "control-plane" {
		t.Fatalf("name=%q", cfg.Name)
	}
}

func TestRunElectionWithAcquireRenewLose(t *testing.T) {
	var acquireN, renewN atomic.Int32
	held := atomic.Bool{}
	held.Store(true)

	ops := LeaseOps{
		Acquire: func(ctx context.Context, name, holderID string, ttl time.Duration) (bool, error) {
			acquireN.Add(1)
			if name != "control-plane" || holderID != "h1" {
				t.Fatalf("acquire args name=%q holder=%q", name, holderID)
			}
			return held.Load(), nil
		},
		Renew: func(ctx context.Context, name, holderID string, ttl time.Duration) (bool, error) {
			renewN.Add(1)
			return held.Load(), nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var leaderStarts atomic.Int32
	var leaderStops atomic.Int32
	done := make(chan struct{})

	go RunElectionWith(ctx, ops, ElectionConfig{
		Name:       "control-plane",
		HolderID:   "h1",
		TTL:        200 * time.Millisecond,
		RenewEvery: 40 * time.Millisecond,
		RetryEvery: 40 * time.Millisecond,
	}, nil, func(leaderCtx context.Context) {
		leaderStarts.Add(1)
		<-leaderCtx.Done()
		leaderStops.Add(1)
		if leaderStops.Load() == 1 {
			close(done)
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	for leaderStarts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if leaderStarts.Load() == 0 {
		t.Fatal("expected leader start")
	}

	held.Store(false)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected leader stop after lease loss")
	}

	held.Store(true)
	deadline = time.Now().Add(2 * time.Second)
	for leaderStarts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if leaderStarts.Load() < 2 {
		t.Fatalf("expected reacquire, starts=%d", leaderStarts.Load())
	}
	if renewN.Load() == 0 && acquireN.Load() < 2 {
		t.Fatal("expected acquire/renew activity")
	}
	cancel()
}
