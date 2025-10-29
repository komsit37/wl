package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/komsit37/wl/pkg/wl/types"
)

// symsRenderer prints all symbols in a single comma-separated line.
type symsRenderer struct{}

func NewSymsRenderer() Renderer {
	return symsRenderer{}
}

func (symsRenderer) Render(w io.Writer, lists []types.Watchlist, _ RenderOptions) error {
	symbols := make([]string, 0)
	for _, list := range lists {
		for _, item := range list.Items {
			sym := strings.TrimSpace(item.Sym)
			if sym == "" {
				continue
			}
			symbols = append(symbols, strings.TrimSuffix(sym, ".T"))
		}
	}
	_, err := fmt.Fprintln(w, strings.Join(symbols, ","))
	return err
}
