package recent

import (
	"context"

	"github.com/bjarneo/cliamp/history"
)

// HistorySource adapts cliamp's persistent local history to Source.
type HistorySource struct {
	Store *history.Store
}

func (s HistorySource) Recent(ctx context.Context, limit int) ([]Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Store == nil {
		return nil, nil
	}
	entries, err := s.Store.Recent(limit)
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(entries))
	for i, entry := range entries {
		items[i] = Item{Track: entry.Track, PlayedAt: entry.PlayedAt, Sources: []string{"cliamp"}}
	}
	return items, nil
}
