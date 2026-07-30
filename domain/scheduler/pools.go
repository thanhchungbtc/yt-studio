package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/semaphore"

	"github.com/tbui/yt-studio/domain/entity"
)

// ErrUnknownPool is returned when a pool name is not one of the constants.
var ErrUnknownPool = errors.New("unknown pool")

// ErrPoolLimitOutOfRange is returned when a requested limit is unusable.
var ErrPoolLimitOutOfRange = errors.New("pool limit out of range")

// Pools is the global admission control. Limits are enforced across all videos
// and all channels; a task acquires exactly one slot in exactly one pool and
// holds it for the duration of the provider call.
//
// The mechanism is golang.org/x/sync/semaphore.Weighted — context-cancellable
// and FIFO-fair. We write no counter of our own.
//
// Runtime limit changes (a settings row edit, applied without a restart) are
// implemented as ballast: every semaphore is created at MaxPoolLimit capacity
// and the difference between that and the effective limit is held as tokens.
// Lowering a limit therefore takes effect as running tasks release their slots,
// which is the only correct semantics — a running provider call is not
// preemptible.
type Pools struct {
	sems     [entity.NumPools]*semaphore.Weighted
	limits   [entity.NumPools]atomic.Int64
	inFlight [entity.NumPools]atomic.Int64

	mu      sync.Mutex
	ballast [entity.NumPools]int64

	// requests carries desired limits to the per-pool reconcilers, so an API call
	// never blocks behind a running provider call.
	requests [entity.NumPools]chan int64
}

// NewPools creates the pools with the given effective limits. A zero or missing
// limit falls back to the seeded default for that pool.
func NewPools(limits map[entity.Pool]int) (*Pools, error) {
	p := &Pools{}
	for i, pool := range entity.AllPools {
		n := limits[pool]
		if n == 0 {
			n = entity.DefaultPoolLimit(pool)
		}
		if n < 1 || n > entity.MaxPoolLimit {
			return nil, fmt.Errorf("%w: %s=%d, must be 1..%d", ErrPoolLimitOutOfRange, pool, n, entity.MaxPoolLimit)
		}
		sem := semaphore.NewWeighted(entity.MaxPoolLimit)
		ballast := int64(entity.MaxPoolLimit - n)
		if ballast > 0 && !sem.TryAcquire(ballast) {
			// Unreachable: the semaphore is untouched at this point.
			return nil, fmt.Errorf("%w: could not reserve ballast for %s", ErrPoolLimitOutOfRange, pool)
		}
		p.sems[i] = sem
		p.ballast[i] = ballast
		p.limits[i].Store(int64(n))
		p.requests[i] = make(chan int64, 1)
	}
	return p, nil
}

// TryAcquire takes one slot without blocking. It is on the dispatch hot path
// and allocates nothing.
func (p *Pools) TryAcquire(pool entity.Pool) bool {
	i := pool.Index()
	if i < 0 {
		return false
	}
	if !p.sems[i].TryAcquire(1) {
		return false
	}
	p.inFlight[i].Add(1)
	return true
}

// Release returns one slot.
func (p *Pools) Release(pool entity.Pool) {
	i := pool.Index()
	if i < 0 {
		return
	}
	p.inFlight[i].Add(-1)
	p.sems[i].Release(1)
}

// Limit reports the effective limit of a pool.
func (p *Pools) Limit(pool entity.Pool) int {
	i := pool.Index()
	if i < 0 {
		return 0
	}
	return int(p.limits[i].Load())
}

// InFlight reports how many slots of a pool are currently held by tasks.
func (p *Pools) InFlight(pool entity.Pool) int {
	i := pool.Index()
	if i < 0 {
		return 0
	}
	return int(p.inFlight[i].Load())
}

// TotalInFlight reports slots held across every pool.
func (p *Pools) TotalInFlight() int {
	total := 0
	for i := range p.inFlight {
		total += int(p.inFlight[i].Load())
	}
	return total
}

// SetLimit requests a new effective limit. It never blocks: raising a limit is
// immediate, lowering one is handed to that pool's reconciler and completes as
// running tasks finish.
func (p *Pools) SetLimit(pool entity.Pool, n int) error {
	i := pool.Index()
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrUnknownPool, pool)
	}
	if n < 1 || n > entity.MaxPoolLimit {
		return fmt.Errorf("%w: %s=%d, must be 1..%d", ErrPoolLimitOutOfRange, pool, n, entity.MaxPoolLimit)
	}
	select {
	case p.requests[i] <- int64(n):
	default:
		// A request is already queued; replace it so the newest value wins.
		select {
		case <-p.requests[i]:
		default:
		}
		select {
		case p.requests[i] <- int64(n):
		default:
		}
	}
	return nil
}

// Run owns the per-pool reconcilers. It returns when ctx is done; every
// goroutine it starts has exited by then.
func (p *Pools) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for i := range entity.AllPools {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.reconcile(ctx, i)
		}(i)
	}
	wg.Wait()
	return ctx.Err()
}

func (p *Pools) reconcile(ctx context.Context, i int) {
	for {
		select {
		case <-ctx.Done():
			return
		case want := <-p.requests[i]:
			p.applyLimit(ctx, i, want)
		}
	}
}

// applyLimit moves ballast so that the effective capacity becomes want.
// Acquiring ballast can block until running tasks release their slots; that is
// the point, and it happens on the reconciler goroutine, never on a caller's.
func (p *Pools) applyLimit(ctx context.Context, i int, want int64) {
	p.mu.Lock()
	cur := p.limits[i].Load()
	p.mu.Unlock()
	if want == cur {
		return
	}
	if want > cur {
		delta := want - cur
		p.mu.Lock()
		if delta > p.ballast[i] {
			delta = p.ballast[i]
		}
		if delta > 0 {
			p.sems[i].Release(delta)
			p.ballast[i] -= delta
			p.limits[i].Add(delta)
		}
		p.mu.Unlock()
		return
	}
	delta := cur - want
	if err := p.sems[i].Acquire(ctx, delta); err != nil {
		return // shutting down
	}
	p.mu.Lock()
	p.ballast[i] += delta
	p.limits[i].Add(-delta)
	p.mu.Unlock()
}
