package types

// Watchlist represents a named list with optional explicit column order.
type Watchlist struct {
	Name    string
	Columns []string
	Items   []Item
}

// Item represents a symbol entry and arbitrary fields.
// Fields may be used to store precomputed values for rendering.
type Item struct {
	Sym    string
	Name   string
	Fields map[string]any
}

// Quote contains formatted and raw change values for rendering.
type Quote struct {
	Price  string
	ChgFmt string
	ChgRaw float64
	Name   string
}

// Fundamentals is a minimal set of fundamentals used by columns.
type Fundamentals struct {
	Exchange string
	Industry string
	PE       *float64
	ROE      *float64 // percent value, e.g., 12.3 for 12.3%
	// AssetProfile-derived fields
	Sector          string
	Employees       int
	Address1        string
	City            string
	Country         string
	Zip             string
	Phone           string
	Website         string
	IR              string
	BusinessSummary string
	OfficersCount   int
	AvgOfficerAge   *float64
	CEOName         string
	CEOTitle        string
	CEOAge          *int
}
