package gql

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/server/biz"
)

const usagePriceBatchWindow = 5 * time.Millisecond

type usagePriceLoaderKey struct{}

type usagePriceResult struct {
	multiplier *decimal.Decimal
	err        error
}

// usagePriceLoader lives for one GraphQL operation, so sibling usage logs share
// one price-version lookup without retaining prices across requests. The shared
// query runs on the operation context; a caller's context only bounds its wait.
type usagePriceLoader struct {
	ctx    context.Context
	client *ent.Client
	// schedule arms the batch window and calls fire once it closes.
	schedule func(fire func())

	mu       sync.Mutex
	pending  map[string][]chan usagePriceResult
	inflight map[string][]chan usagePriceResult
	cache    map[string]usagePriceResult
}

func newUsagePriceLoader(ctx context.Context, client *ent.Client) *usagePriceLoader {
	return &usagePriceLoader{
		ctx:      ctx,
		client:   client,
		schedule: func(fire func()) { time.AfterFunc(usagePriceBatchWindow, fire) },
		cache:    make(map[string]usagePriceResult),
	}
}

func withUsagePriceLoader(ctx context.Context, client *ent.Client) context.Context {
	return context.WithValue(ctx, usagePriceLoaderKey{}, newUsagePriceLoader(ctx, client))
}

func (loader *usagePriceLoader) load(ctx context.Context, referenceID string) (*decimal.Decimal, error) {
	if referenceID == "" {
		return nil, nil
	}

	ch := make(chan usagePriceResult, 1)

	loader.mu.Lock()
	if cached, ok := loader.cache[referenceID]; ok {
		loader.mu.Unlock()
		return cached.multiplier, nil
	}

	if waiters, ok := loader.inflight[referenceID]; ok {
		loader.inflight[referenceID] = append(waiters, ch)
		loader.mu.Unlock()

		return awaitUsagePrice(ctx, ch)
	}

	opensBatch := len(loader.pending) == 0
	if loader.pending == nil {
		loader.pending = make(map[string][]chan usagePriceResult)
	}

	loader.pending[referenceID] = append(loader.pending[referenceID], ch)
	loader.mu.Unlock()

	if opensBatch {
		loader.schedule(loader.flush)
	}

	return awaitUsagePrice(ctx, ch)
}

func awaitUsagePrice(ctx context.Context, ch <-chan usagePriceResult) (*decimal.Decimal, error) {
	select {
	case result := <-ch:
		return result.multiplier, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// flush runs on the timer goroutine started by schedule.
func (loader *usagePriceLoader) flush() {
	defer func() {
		if r := recover(); r != nil {
			log.Error(loader.ctx, "usage price batch flush panicked", log.Any("panic", r))
		}
	}()

	loader.mu.Lock()
	pending := loader.pending
	loader.pending = nil

	if len(pending) == 0 {
		loader.mu.Unlock()
		return
	}

	if loader.inflight == nil {
		loader.inflight = make(map[string][]chan usagePriceResult, len(pending))
	}

	refs := make([]string, 0, len(pending))
	for ref, waiters := range pending {
		loader.inflight[ref] = waiters
		refs = append(refs, ref)
	}
	loader.mu.Unlock()

	prices, err := loader.query(refs)

	loader.mu.Lock()
	defer loader.mu.Unlock()

	for _, ref := range refs {
		result := usagePriceResult{err: err}
		if multiplier, ok := prices[ref]; ok {
			result.multiplier = &multiplier
		}
		// Errors are not cached, so a later field in the same operation may retry.
		if err == nil {
			loader.cache[ref] = result
		}

		for _, waiter := range loader.inflight[ref] {
			waiter <- result
		}

		delete(loader.inflight, ref)
	}
}

// query converts a panic into an error so every waiter of the batch still receives a result.
func (loader *usagePriceLoader) query(refs []string) (prices map[string]decimal.Decimal, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(loader.ctx, "usage price multiplier query panicked", log.Any("panic", r))
			prices, err = nil, fmt.Errorf("load usage price multipliers: panic: %v", r)
		}
	}()

	return biz.LoadUsagePriceMultipliers(loader.ctx, loader.client, refs)
}
