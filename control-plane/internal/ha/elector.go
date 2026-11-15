package ha

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ElectionConfig controls Postgres-backed leader election (S17).
type ElectionConfig struct {
	Name        string
	HolderID    string
	TTL         time.Duration
	RenewEvery  time.Duration
	RetryEvery  time.Duration // when not leader
}

// Normalize fills defaults: TTL 15s, renew every TTL/3, retry every renew.
func (c ElectionConfig) Normalize() ElectionConfig {
	if c.TTL <= 0 {
		c.TTL = DefaultLeaseTTL()
	}
	if c.RenewEvery <= 0 {
		c.RenewEvery = c.TTL / 3
		if c.RenewEvery < time.Second {
			c.RenewEvery = time.Second
		}
	}
	if c.RenewEvery >= c.TTL {
		c.RenewEvery = c.TTL / 3
		if c.RenewEvery < time.Second {
			c.RenewEvery = time.Second
		}
	}
	if c.RetryEvery <= 0 {
		c.RetryEvery = c.RenewEvery
	}
	if c.Name == "" {
		c.Name = "control-plane"
	}
	return c
}

// LeaseOps abstracts Acquire/Renew for tests.
type LeaseOps struct {
	Acquire func(ctx context.Context, name, holderID string, ttl time.Duration) (bool, error)
	Renew   func(ctx context.Context, name, holderID string, ttl time.Duration) (bool, error)
}

// PoolLeaseOps binds AcquireLease/RenewLease to a pool.
func PoolLeaseOps(pool *pgxpool.Pool) LeaseOps {
	return LeaseOps{
		Acquire: func(ctx context.Context, name, holderID string, ttl time.Duration) (bool, error) {
			return AcquireLease(ctx, pool, name, holderID, ttl)
		},
		Renew: func(ctx context.Context, name, holderID string, ttl time.Duration) (bool, error) {
			return RenewLease(ctx, pool, name, holderID, ttl)
		},
	}
}

// RunElection acquires/renews a lease and runs onLeader only while holding it.
// HTTP and other non-leader work stay outside this function. When the lease is
// lost, onLeader's context is cancelled; when reacquired, onLeader starts again.
func RunElection(ctx context.Context, pool *pgxpool.Pool, cfg ElectionConfig, logger *slog.Logger, onLeader func(context.Context)) {
	RunElectionWith(ctx, PoolLeaseOps(pool), cfg, logger, onLeader)
}

// RunElectionWith is the testable election loop.
func RunElectionWith(ctx context.Context, ops LeaseOps, cfg ElectionConfig, logger *slog.Logger, onLeader func(context.Context)) {
	cfg = cfg.Normalize()
	if logger == nil {
		logger = slog.Default()
	}
	if ops.Acquire == nil || ops.Renew == nil {
		logger.Error("ha election missing lease ops")
		return
	}
	if cfg.HolderID == "" {
		logger.Error("ha election missing holder_id")
		return
	}
	if onLeader == nil {
		onLeader = func(context.Context) {}
	}

	var (
		leaderCancel context.CancelFunc
		isLeader     bool
	)
	stopLeader := func() {
		if leaderCancel != nil {
			leaderCancel()
			leaderCancel = nil
		}
		if isLeader {
			logger.Info("ha lease lost; stopping leader work", "name", cfg.Name, "holder", cfg.HolderID)
			isLeader = false
		}
	}
	startLeader := func() {
		if isLeader {
			return
		}
		leaderCtx, cancel := context.WithCancel(ctx)
		leaderCancel = cancel
		isLeader = true
		logger.Info("ha lease acquired; starting leader work", "name", cfg.Name, "holder", cfg.HolderID, "ttl", cfg.TTL.String())
		go onLeader(leaderCtx)
	}

	ticker := time.NewTicker(cfg.RetryEvery)
	defer ticker.Stop()
	defer stopLeader()

	tryOnce := func() {
		if ctx.Err() != nil {
			return
		}
		if isLeader {
			ok, err := ops.Renew(ctx, cfg.Name, cfg.HolderID, cfg.TTL)
			if err != nil {
				logger.Error("ha lease renew failed", "name", cfg.Name, "error", err)
				stopLeader()
				return
			}
			if !ok {
				stopLeader()
				return
			}
			return
		}
		ok, err := ops.Acquire(ctx, cfg.Name, cfg.HolderID, cfg.TTL)
		if err != nil {
			logger.Error("ha lease acquire failed", "name", cfg.Name, "error", err)
			return
		}
		if ok {
			startLeader()
			ticker.Reset(cfg.RenewEvery)
		}
	}

	tryOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			wasLeader := isLeader
			tryOnce()
			if wasLeader && !isLeader {
				ticker.Reset(cfg.RetryEvery)
			} else if !wasLeader && isLeader {
				ticker.Reset(cfg.RenewEvery)
			}
		}
	}
}
