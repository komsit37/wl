package columns

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	yfgo "github.com/komsit37/yf-go"

	"github.com/komsit37/wl/pkg/wl/types"
)

// ColumnDef describes a column mapping to a Yahoo module and JSON path.
type ColumnDef struct {
	Key     string
	Aliases []string
	Module  yfgo.QuoteSummaryModule
	Path    string // dot path with '|' fallbacks, terminal len() for arrays
}

var (
	defsByKey  = map[string]ColumnDef{}
	aliasToKey = map[string]string{}
)

// RegisterDef registers a column definition and its aliases.
func RegisterDef(def ColumnDef) {
	k := strings.ToLower(def.Key)
	defsByKey[k] = def
	aliasToKey[k] = k
	for _, a := range def.Aliases {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" {
			aliasToKey[a] = k
		}
	}
}

// Canonical resolves a provided name to its canonical key.
func Canonical(col string) (string, bool) {
	lc := strings.ToLower(strings.TrimSpace(col))
	if lc == "" {
		return "", false
	}
	if k, ok := aliasToKey[lc]; ok {
		return k, true
	}
	return lc, false
}

// AvailableByModule returns canonical columns grouped by module name.
func AvailableByModule() map[string][]string {
	groups := map[string][]string{}
	for k, def := range defsByKey {
		grp := string(def.Module)
		if grp == "" {
			grp = "base"
		}
		groups[grp] = append(groups[grp], k)
	}
	for grp := range groups {
		sort.Strings(groups[grp])
	}
	return groups
}

// registerMeta defines all built-in columns in one place.
func registerMeta() {
	// Base
	RegisterDef(ColumnDef{Key: "sym"})
	RegisterDef(ColumnDef{Key: "name", Module: yfgo.ModulePrice, Path: "price.shortName|price.longName"})

	// Price
	RegisterDef(ColumnDef{Key: "price", Module: yfgo.ModulePrice, Path: "price.regularMarketPrice.fmt"})
	RegisterDef(ColumnDef{Key: "chg%", Module: yfgo.ModulePrice, Path: "price.regularMarketChangePercent.fmt"})

	// AssetProfile
	RegisterDef(ColumnDef{Key: "sector", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.sector"})
	RegisterDef(ColumnDef{Key: "industry", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.industry"})
	RegisterDef(ColumnDef{Key: "employees", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.fullTimeEmployees"})
	RegisterDef(ColumnDef{Key: "website", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.website"})
	RegisterDef(ColumnDef{Key: "ir", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.irWebsite"})
	RegisterDef(ColumnDef{Key: "officers_count", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.companyOfficers.len()"})
	RegisterDef(ColumnDef{Key: "avg_officer_age", Module: yfgo.ModuleAssetProfile}) // derived
	RegisterDef(ColumnDef{Key: "business_summary", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.longBusinessSummary"})
	RegisterDef(ColumnDef{Key: "hq", Module: yfgo.ModuleAssetProfile})  // derived
	RegisterDef(ColumnDef{Key: "ceo", Module: yfgo.ModuleAssetProfile}) // derived
	RegisterDef(ColumnDef{Key: "address1", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.address1"})
	RegisterDef(ColumnDef{Key: "city", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.city"})
	RegisterDef(ColumnDef{Key: "zip", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.zip"})
	RegisterDef(ColumnDef{Key: "country", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.country"})
	RegisterDef(ColumnDef{Key: "phone", Module: yfgo.ModuleAssetProfile, Path: "assetProfile.phone"})

	// FinancialData
	RegisterDef(ColumnDef{Key: "cr", Module: yfgo.ModuleFinancialData, Path: "financialData.currentRatio.fmt"})
	RegisterDef(ColumnDef{Key: "qr", Module: yfgo.ModuleFinancialData, Path: "financialData.quickRatio.fmt"})
	RegisterDef(ColumnDef{Key: "de%", Module: yfgo.ModuleFinancialData, Path: "financialData.debtToEquity.fmt"})
	RegisterDef(ColumnDef{Key: "roa%", Module: yfgo.ModuleFinancialData, Path: "financialData.returnOnAssets.fmt"})
	RegisterDef(ColumnDef{Key: "roe%", Module: yfgo.ModuleFinancialData, Path: "financialData.returnOnEquity.fmt"})
	RegisterDef(ColumnDef{Key: "pm%", Module: yfgo.ModuleFinancialData, Path: "financialData.profitMargins.fmt"})
	RegisterDef(ColumnDef{Key: "om%", Module: yfgo.ModuleFinancialData, Path: "financialData.operatingMargins.fmt"})
	RegisterDef(ColumnDef{Key: "gm%", Module: yfgo.ModuleFinancialData, Path: "financialData.grossMargins.fmt"})
	RegisterDef(ColumnDef{Key: "rev_g%", Module: yfgo.ModuleFinancialData, Path: "financialData.revenueGrowth.fmt"})
	RegisterDef(ColumnDef{Key: "earn_g%", Module: yfgo.ModuleFinancialData, Path: "financialData.earningsGrowth.fmt"})
	RegisterDef(ColumnDef{Key: "rev_ps", Module: yfgo.ModuleFinancialData, Path: "financialData.revenuePerShare.fmt"})
	RegisterDef(ColumnDef{Key: "cash", Module: yfgo.ModuleFinancialData, Path: "financialData.totalCash.fmt"})
	RegisterDef(ColumnDef{Key: "debt", Module: yfgo.ModuleFinancialData, Path: "financialData.totalDebt.fmt"})
	RegisterDef(ColumnDef{Key: "fcf", Module: yfgo.ModuleFinancialData, Path: "financialData.freeCashflow.fmt"})
	RegisterDef(ColumnDef{Key: "ocf", Module: yfgo.ModuleFinancialData, Path: "financialData.operatingCashflow.fmt"})
	RegisterDef(ColumnDef{Key: "tgt_mean", Module: yfgo.ModuleFinancialData, Path: "financialData.targetMeanPrice.fmt"})
	RegisterDef(ColumnDef{Key: "reco", Module: yfgo.ModuleFinancialData, Path: "financialData.recommendationKey"})
	RegisterDef(ColumnDef{Key: "analysts", Module: yfgo.ModuleFinancialData, Path: "financialData.numberOfAnalystOpinions.raw"})

	// SummaryDetail
	RegisterDef(ColumnDef{Key: "mktcap", Aliases: []string{"marketcap", "MarketCap", "market_cap"}, Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.marketCap.fmt|price.marketCap.fmt"})
	RegisterDef(ColumnDef{Key: "beta", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.beta.fmt"})
	RegisterDef(ColumnDef{Key: "div_yield%", Aliases: []string{"div%"}, Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.dividendYield.fmt"})
	RegisterDef(ColumnDef{Key: "div_rate", Aliases: []string{"div"}, Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.dividendRate.fmt"})
	RegisterDef(ColumnDef{Key: "payout%", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.payoutRatio.fmt"})
	RegisterDef(ColumnDef{Key: "pe_ttm", Aliases: []string{"pe"}, Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.trailingPE.fmt"})
	RegisterDef(ColumnDef{Key: "pe_fwd", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.forwardPE.fmt"})
	RegisterDef(ColumnDef{Key: "ps_ttm", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.priceToSalesTrailing12Months.fmt"})
	RegisterDef(ColumnDef{Key: "avg_vol", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.averageVolume.fmt|summaryDetail.volume.fmt"})
	RegisterDef(ColumnDef{Key: "avg_vol10d", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.averageDailyVolume10Day.fmt|summaryDetail.averageVolume10days.fmt"})
	RegisterDef(ColumnDef{Key: "vol", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.regularMarketVolume.fmt|summaryDetail.volume.fmt"})
	RegisterDef(ColumnDef{Key: "open", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.open.fmt"})
	RegisterDef(ColumnDef{Key: "prev_close", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.previousClose.fmt"})
	RegisterDef(ColumnDef{Key: "50d_avg", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.fiftyDayAverage.fmt"})
	RegisterDef(ColumnDef{Key: "200d_avg", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.twoHundredDayAverage.fmt"})
	RegisterDef(ColumnDef{Key: "day_high", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.dayHigh.fmt"})
	RegisterDef(ColumnDef{Key: "day_low", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.dayLow.fmt"})
	RegisterDef(ColumnDef{Key: "52w_high", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.fiftyTwoWeekHigh.fmt"})
	RegisterDef(ColumnDef{Key: "52w_low", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.fiftyTwoWeekLow.fmt"})
	RegisterDef(ColumnDef{Key: "ath", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.allTimeHigh.fmt"})
	RegisterDef(ColumnDef{Key: "atl", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.allTimeLow.fmt"})
	RegisterDef(ColumnDef{Key: "ex_div", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.exDividendDate.fmt"})
	RegisterDef(ColumnDef{Key: "5y_avg_div_yield", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.fiveYearAvgDividendYield.fmt"})
	RegisterDef(ColumnDef{Key: "ccy", Module: yfgo.ModuleSummaryDetail, Path: "summaryDetail.currency.fmt"})
}

func init() {
	registerMeta()
	BuildDefaultSetsFromDefs()
}

// GetDef returns a column definition by canonical key.
func GetDef(key string) (ColumnDef, bool) {
	def, ok := defsByKey[key]
	return def, ok
}

// FormatFloat prints float with fixed decimals.
func FormatFloat(v float64, decimals int) string {
	return fmt.Sprintf("%."+strconv.Itoa(decimals)+"f", v)
}

// RawToMap converts yf-go raw to a map for path extraction.
func RawToMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// Extract gets a string for a dot path with fallbacks separated by '|'.
// Supports terminal len() to get array length.
func Extract(m map[string]any, path string) (string, bool) {
	if m == nil || strings.TrimSpace(path) == "" {
		return "", false
	}
	for _, alt := range strings.Split(path, "|") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		if v, ok := walkOnce(m, alt); ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return t, true
				}
			case json.Number:
				return t.String(), true
			default:
				return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(stringify(t), "\""), "\"")), true
			}
		}
	}
	return "", false
}

func stringify(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func walkOnce(m map[string]any, path string) (any, bool) {
	cur := any(m)
	parts := strings.Split(path, ".")
	for i, p := range parts {
		if p == "len()" {
			if arr, ok := cur.([]any); ok {
				return float64(len(arr)), true
			}
			return nil, false
		}
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := mm[p]
		if !ok {
			return nil, false
		}
		if i == len(parts)-1 {
			return v, true
		}
		cur = v
	}
	return cur, true
}

// RequiredModules returns unique yf-go modules for the given columns.
func RequiredModules(cols []string) []yfgo.QuoteSummaryModule {
	set := map[yfgo.QuoteSummaryModule]struct{}{}
	for _, c := range cols {
		if k, ok := Canonical(c); ok {
			if def, ok := defsByKey[k]; ok {
				if def.Module != "" {
					set[def.Module] = struct{}{}
				}
			}
		}
	}
	order := []yfgo.QuoteSummaryModule{yfgo.ModulePrice, yfgo.ModuleAssetProfile, yfgo.ModuleFinancialData, yfgo.ModuleSummaryDetail}
	out := make([]yfgo.QuoteSummaryModule, 0, len(set))
	for _, o := range order {
		if _, ok := set[o]; ok {
			out = append(out, o)
		}
	}
	return out
}

// Compute determines final column order from explicit list or inferred from item fields.
func Compute(explicit []string, items []types.Item) []string {
	if len(explicit) > 0 {
		seen := map[string]struct{}{}
		out := make([]string, 0, len(explicit))
		for _, k := range explicit {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
		return out
	}
	set := map[string]struct{}{}
	for _, it := range items {
		for k := range it.Fields {
			set[k] = struct{}{}
		}
		if it.Sym != "" {
			set["sym"] = struct{}{}
		}
		if it.Name != "" {
			set["name"] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	if _, ok := set["sym"]; ok {
		keys = append(keys, "sym")
		delete(set, "sym")
	}
	rest := make([]string, 0, len(set))
	for k := range set {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	keys = append(keys, rest...)

	// ensure name/price/chg% after sym when inferred
	symIdx := -1
	for i, k := range keys {
		if k == "sym" {
			symIdx = i
			break
		}
	}
	if symIdx >= 0 {
		if !contains(keys, "name") {
			keys = insertAfter(keys, symIdx, "name")
		}
		insertAfterIdx := symIdx
		for i, k := range keys {
			if k == "name" {
				insertAfterIdx = i
				break
			}
		}
		if !contains(keys, "price") {
			keys = insertAfter(keys, insertAfterIdx, "price")
		}
		priceIdx := indexOf(keys, "price")
		if priceIdx >= 0 && !contains(keys, "chg%") {
			keys = insertAfter(keys, priceIdx, "chg%")
		}
	}
	return keys
}

func contains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

func indexOf(s []string, v string) int {
	for i, e := range s {
		if e == v {
			return i
		}
	}
	return -1
}

func insertAfter(s []string, idx int, v string) []string {
	if idx < 0 || idx >= len(s) {
		return append(s, v)
	}
	s = append(s, "")
	copy(s[idx+2:], s[idx+1:])
	s[idx+1] = v
	return s
}
