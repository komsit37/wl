package enrich

import (
	"context"
	"fmt"
	"sync"
	"time"

	yfgo "github.com/komsit37/yf-go"

	"github.com/komsit37/wl/pkg/wl/types"
)

// NeedMask declares which data is required for a fetch.
type NeedMask uint64

const (
	NeedNone     NeedMask = 0
	NeedPrice    NeedMask = 1 << iota // price
	NeedChgPct                        // change percent
	NeedExchange                      // exchange
	NeedIndustry                      // industry
	NeedPE                            // price/earnings
	NeedROE                           // return on equity percent
)

// QuoteService fetches quote and fundamentals for a symbol.
type QuoteService interface {
	Get(ctx context.Context, sym string, need NeedMask) (types.Quote, types.Fundamentals, error)
}

// YFService implements QuoteService using yf-go.
type YFService struct {
	client  *yfgo.Client
	timeout time.Duration
}

func NewYFService(timeout time.Duration) *YFService {
	return &YFService{client: yfgo.NewClient(), timeout: timeout}
}

func (s *YFService) Get(ctx context.Context, sym string, need NeedMask) (types.Quote, types.Fundamentals, error) {
	if sym == "" {
		return types.Quote{}, types.Fundamentals{}, nil
	}
	// Currently only request ModulePrice; other modules can be wired later.
	mods := []yfgo.QuoteSummaryModule{yfgo.ModulePrice}
	// In future: append modules based on need mask (e.g., SummaryProfile, DefaultKeyStatistics)

	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	res, err := s.client.QuoteSummaryTyped(cctx, sym, mods)
	if err != nil {
		return types.Quote{}, types.Fundamentals{}, err
	}
	if res.Price == nil {
		return types.Quote{}, types.Fundamentals{}, fmt.Errorf("no price for %s", sym)
	}

	var q types.Quote
	// Price
	p := res.Price.RegularMarketPrice
	if p.Fmt != "" {
		q.Price = p.Fmt
	} else if p.Raw != nil {
		q.Price = fmt.Sprintf("%.2f", *p.Raw)
	}
	// Change percent
	cp := res.Price.RegularMarketChangePercent
	if cp.Fmt != "" {
		q.ChgFmt = cp.Fmt
	}
	if cp.Raw != nil {
		q.ChgRaw = *cp.Raw
		if q.ChgFmt == "" {
			q.ChgFmt = fmt.Sprintf("%.2f%%", q.ChgRaw)
		}
	}
	// Name
	if res.Price.ShortName != "" {
		q.Name = res.Price.ShortName
	} else if res.Price.LongName != "" {
		q.Name = res.Price.LongName
	}

	// Fundamentals: currently not populated; stubs for future expansion.
	var f types.Fundamentals
	return q, f, nil
}

// CacheService decorates a QuoteService with TTL+LRU cache.
type CacheService struct {
	next QuoteService
	ttl  time.Duration
	size int

	mu    sync.Mutex
	items map[string]cacheEntry
	order []string // simple LRU order, oldest at index 0
}

type cacheEntry struct {
	at   time.Time
	q    types.Quote
	fund types.Fundamentals
}

func NewCacheService(next QuoteService, ttl time.Duration, size int) *CacheService {
	return &CacheService{next: next, ttl: ttl, size: size, items: make(map[string]cacheEntry)}
}

func (c *CacheService) key(sym string, need NeedMask) string {
	return fmt.Sprintf("%s|%d", sym, need)
}

func (c *CacheService) Get(ctx context.Context, sym string, need NeedMask) (types.Quote, types.Fundamentals, error) {
	if sym == "" {
		return types.Quote{}, types.Fundamentals{}, nil
	}
	k := c.key(sym, need)
	now := time.Now()
	c.mu.Lock()
	if ent, ok := c.items[k]; ok {
		if now.Sub(ent.at) <= c.ttl {
			// refresh LRU
			c.touchLocked(k)
			q, f := ent.q, ent.fund
			c.mu.Unlock()
			return q, f, nil
		}
		// expired; drop and continue
		delete(c.items, k)
		c.removeFromOrderLocked(k)
	}
	c.mu.Unlock()

	// Delegate
	q, f, err := c.next.Get(ctx, sym, need)
	if err != nil {
		return q, f, err
	}
	c.mu.Lock()
	c.items[k] = cacheEntry{at: now, q: q, fund: f}
	c.order = append(c.order, k)
	// Enforce size
	for len(c.items) > c.size && len(c.order) > 0 {
		old := c.order[0]
		c.order = c.order[1:]
		delete(c.items, old)
	}
	c.mu.Unlock()
	return q, f, nil
}

func (c *CacheService) touchLocked(k string) {
	// move key to end
	for i, v := range c.order {
		if v == k {
			c.order = append(append(c.order[:i], c.order[i+1:]...), k)
			return
		}
	}
	c.order = append(c.order, k)
}

func (c *CacheService) removeFromOrderLocked(k string) {
	for i, v := range c.order {
		if v == k {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}
