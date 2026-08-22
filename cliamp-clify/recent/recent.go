// Package recent merges timestamped listening history from independent
// providers without depending on the TUI.
package recent

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bjarneo/cliamp/playlist"
)

const (
	SourcesMetaKey  = "clify.sources"
	PlayedAtMetaKey = "clify.played_at"
)

type Item struct {
	Track    playlist.Track
	PlayedAt time.Time
	Sources  []string
}

type Source interface {
	Recent(ctx context.Context, limit int) ([]Item, error)
}

// UnifiedSource is implemented by providers that expose an already-merged
// local and remote recent-history view.
type UnifiedSource interface {
	UnifiedRecent(ctx context.Context, limit int) Result
}

type NamedItems struct {
	Name  string
	Items []Item
	Err   error
}

type Result struct {
	Items         []Item
	Partial       bool
	FailedSources []string
}

// Merge returns newest-first items, collapsing canonical Spotify URIs and
// stable provider IDs. Source slices and their metadata maps are never mutated.
func Merge(limit int, sources ...NamedItems) Result {
	result := Result{}
	byKey := make(map[string]int)

	for _, source := range sources {
		name := strings.TrimSpace(source.Name)
		if source.Err != nil {
			result.Partial = true
			if name != "" {
				result.FailedSources = append(result.FailedSources, name)
			}
			continue
		}
		for _, input := range source.Items {
			item := cloneItem(input)
			item.Sources = unionStrings(item.Sources, []string{name})
			key := canonicalKey(item.Track)
			if key == "" {
				result.Items = append(result.Items, item)
				continue
			}
			if index, ok := byKey[key]; ok {
				existing := result.Items[index]
				combinedSources := unionStrings(existing.Sources, item.Sources)
				if newer(item.PlayedAt, existing.PlayedAt) {
					item.Sources = combinedSources
					result.Items[index] = item
				} else {
					existing.Sources = combinedSources
					result.Items[index] = existing
				}
				continue
			}
			byKey[key] = len(result.Items)
			result.Items = append(result.Items, item)
		}
	}

	sort.SliceStable(result.Items, func(i, j int) bool {
		return newer(result.Items[i].PlayedAt, result.Items[j].PlayedAt)
	})
	for i := range result.Items {
		setMetadata(&result.Items[i])
	}
	slices.Sort(result.FailedSources)
	result.FailedSources = slices.Compact(result.FailedSources)
	if limit > 0 && len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result
}

func cloneItem(item Item) Item {
	item.Sources = slices.Clone(item.Sources)
	if item.Track.ProviderMeta != nil {
		item.Track.ProviderMeta = mapsClone(item.Track.ProviderMeta)
	}
	return item
}

func mapsClone(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func canonicalKey(track playlist.Track) string {
	if strings.HasPrefix(strings.ToLower(track.Path), "spotify:track:") {
		return strings.ToLower(track.Path)
	}
	keys := make([]string, 0, len(track.ProviderMeta))
	for key, value := range track.ProviderMeta {
		if value != "" && strings.HasSuffix(strings.ToLower(key), ".id") {
			keys = append(keys, strings.ToLower(key)+"="+value)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	slices.Sort(keys)
	return keys[0]
}

func newer(a, b time.Time) bool {
	if a.IsZero() {
		return false
	}
	if b.IsZero() {
		return true
	}
	return a.After(b)
}

func unionStrings(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, values := range [][]string{a, b} {
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				set[value] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func setMetadata(item *Item) {
	if item.Track.ProviderMeta == nil {
		item.Track.ProviderMeta = make(map[string]string)
	}
	item.Track.ProviderMeta[SourcesMetaKey] = strings.Join(item.Sources, ",")
	if !item.PlayedAt.IsZero() {
		item.Track.ProviderMeta[PlayedAtMetaKey] = item.PlayedAt.UTC().Format(time.RFC3339)
	}
}
