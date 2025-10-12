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
	// FinancialData-derived fields (formatted strings for display)
	Financial Financials
	// SummaryDetail-derived fields (formatted strings for display)
	Detail SummaryDetail
}

// Financials holds selected fields from Yahoo's financialData module (all formatted for display).
type Financials struct {
	CurrentRatio            string
	QuickRatio              string
	DebtToEquity            string
	ReturnOnAssets          string // percent string
	ReturnOnEquity          string // percent string
	EBITDAttMargins         string // percent string (ebitdaMargins)
	OperatingMargins        string // percent string
	ProfitMargins           string // percent string
	GrossMargins            string // percent string
	RevenueGrowth           string // percent string
	EarningsGrowth          string // percent string
	RevenuePerShare         string
	TotalCash               string
	TotalDebt               string
	FreeCashflow            string
	OperatingCashflow       string
	TargetHighPrice         string
	TargetLowPrice          string
	TargetMeanPrice         string
	TargetMedianPrice       string
	RecommendationKey       string
	RecommendationMean      string
	NumberOfAnalystOpinions int
}

// SummaryDetail holds selected fields from Yahoo's summaryDetail module (formatted strings).
type SummaryDetail struct {
	Currency                 string
	MarketCap                string
	Beta                     string
	DividendYield            string
	DividendRate             string
	PayoutRatio              string
	TrailingPE               string
	ForwardPE                string
	PriceToSalesTTM          string
	AverageVolume            string
	AverageDailyVolume10Day  string
	AverageVolume10days      string
	RegularMarketVolume      string
	Volume                   string
	Open                     string
	PreviousClose            string
	FiftyDayAverage          string
	TwoHundredDayAverage     string
	DayHigh                  string
	DayLow                   string
	FiftyTwoWeekHigh         string
	FiftyTwoWeekLow          string
	AllTimeHigh              string
	AllTimeLow               string
	ExDividendDate           string
	FiveYearAvgDividendYield string
}
