package main

import (
	"context"
	"fmt"

	yfgo "github.com/komsit37/yf-go"
)

func main() {
	ctx := context.Background()
	quotes, err := yfgo.DefaultAPI.Quote(ctx, []string{"AAPL"})
	if err != nil || len(quotes) == 0 {
		fmt.Println("error fetching AAPL:", err)
		return
	}
	q := quotes[0]
	if q.RegularMarketPrice != nil {
		fmt.Printf("AAPL: %.2f\n", *q.RegularMarketPrice)
	} else {
		fmt.Println("AAPL: price unavailable")
	}
}
