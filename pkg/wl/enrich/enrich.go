package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	yfgo "github.com/komsit37/yf-go"

	"github.com/komsit37/wl/pkg/wl/types"
)

// NeedMask declares which data is required for a fetch.
type NeedMask uint64

const (
	NeedNone         NeedMask = 0
	NeedPrice        NeedMask = 1 << iota // price
	NeedChgPct                            // change percent
	NeedExchange                          // exchange
	NeedIndustry                          // industry (legacy; maps to assetProfile)
	NeedPE                                // price/earnings
	NeedROE                               // return on equity percent
	NeedAssetProfile                      // assetProfile module (sector, industry, HQ, website, IR, officers)
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
	// Build modules list based on need mask.
	// Always include price because we often surface name and price.
	mods := []yfgo.QuoteSummaryModule{yfgo.ModulePrice}
	// Map legacy NeedIndustry to NeedAssetProfile as source of industry/sector.
	if need&NeedAssetProfile != 0 || need&NeedIndustry != 0 {
		mods = append(mods, yfgo.ModuleAssetProfile)
	}

	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	// Fetch raw to allow decoding additional assetProfile fields (e.g., officers, phone, IR).
	raw, err := s.client.QuoteSummary(cctx, sym, mods)
	if err != nil {
		return types.Quote{}, types.Fundamentals{}, err
	}
	// Decode into typed view for price convenience
	var res yfgo.QuoteSummaryTyped
	if b, ok := rawToJSON(raw); ok {
		_ = json.Unmarshal(b, &res)
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

	// Fundamentals
	var f types.Fundamentals
	// Populate assetProfile-backed fields when requested
	if need&NeedAssetProfile != 0 || need&NeedIndustry != 0 {
		// Define a minimal struct to capture required fields.
		var ap struct {
			AssetProfile struct {
				Sector              string `json:"sector"`
				Industry            string `json:"industry"`
				Website             string `json:"website"`
				IrWebsite           string `json:"irWebsite"`
				LongBusinessSummary string `json:"longBusinessSummary"`
				Address1            string `json:"address1"`
				City                string `json:"city"`
				Country             string `json:"country"`
				Zip                 string `json:"zip"`
				Phone               string `json:"phone"`
				FullTimeEmployees   int64  `json:"fullTimeEmployees"`
				CompanyOfficers     []struct {
					Name  string `json:"name"`
					Title string `json:"title"`
					Age   *int   `json:"age"`
				} `json:"companyOfficers"`
			} `json:"assetProfile"`
		}
		if b, ok := rawToJSON(raw); ok {
			_ = json.Unmarshal(b, &ap)
		}
		f.Sector = ap.AssetProfile.Sector
		f.Industry = ap.AssetProfile.Industry
		if ap.AssetProfile.FullTimeEmployees > 0 {
			f.Employees = int(ap.AssetProfile.FullTimeEmployees)
		}
		f.Address1 = ap.AssetProfile.Address1
		f.City = ap.AssetProfile.City
		f.Country = ap.AssetProfile.Country
		f.Zip = ap.AssetProfile.Zip
		f.Phone = ap.AssetProfile.Phone
		f.Website = ap.AssetProfile.Website
		f.IR = ap.AssetProfile.IrWebsite
		f.BusinessSummary = ap.AssetProfile.LongBusinessSummary
		// Officers
		f.OfficersCount = len(ap.AssetProfile.CompanyOfficers)
		var ageSum int
		var ageCnt int
		// CEO selection by title keywords
		bestIdx := -1
		for i, o := range ap.AssetProfile.CompanyOfficers {
			if o.Title == "" {
				continue
			}
			title := strings.ToLower(o.Title)
			if strings.Contains(title, "ceo") || strings.Contains(title, "president") || strings.Contains(title, "representative director") {
				bestIdx = i
				break
			}
		}
		if bestIdx == -1 && len(ap.AssetProfile.CompanyOfficers) > 0 {
			bestIdx = 0
		}
		if bestIdx >= 0 {
			o := ap.AssetProfile.CompanyOfficers[bestIdx]
			f.CEOName = normalizeSpace(o.Name)
			f.CEOTitle = normalizeSpace(o.Title)
			if o.Age != nil {
				v := *o.Age
				f.CEOAge = &v
			}
		}
		for _, o := range ap.AssetProfile.CompanyOfficers {
			if o.Age != nil {
				ageSum += *o.Age
				ageCnt++
			}
		}
		if ageCnt > 0 {
			avg := float64(ageSum) / float64(ageCnt)
			f.AvgOfficerAge = &avg
		}
	}
	return q, f, nil
}

// rawToJSON marshals an interface{} into JSON bytes.
func rawToJSON(v any) ([]byte, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return b, true
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

// normalizeSpace collapses consecutive whitespace into single spaces and trims ends.
func normalizeSpace(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}
