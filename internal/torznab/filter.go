package torznab

import (
	"strconv"
	"strings"

	"aniliberty-indexer/internal/api"
)

type FilterOptions struct {
	Categories []int
	Season     int
	SeasonSet  bool // filter by Season even when it is 0 (TVDB specials)
	Episode    int
	TVDBID     int
}

func (o FilterOptions) hasSeason() bool {
	return o.SeasonSet || o.Season > 0
}

// Filters combined items by category, season, and episode
func FilterItems(items []api.CombinedItem, opts FilterOptions) []api.CombinedItem {
	if len(opts.Categories) == 0 && !opts.hasSeason() && opts.Episode <= 0 && opts.TVDBID <= 0 {
		return items
	}

	filtered := make([]api.CombinedItem, 0, len(items))
	for _, item := range items {
		if itemMatches(item, opts) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func itemMatches(item api.CombinedItem, opts FilterOptions) bool {
	isMovie := item.Release.Type.Value == "MOVIE"
	res := qualityToResolution(item.Torrent.Quality.Value)

	var itemCats []int
	if isMovie {
		itemCats = resolutionToMovieCategories(res)
	} else {
		itemCats = resolutionToTVCategories(res)
	}

	// Filter by category
	if len(opts.Categories) > 0 {
		catMatch := false
		for _, reqCat := range opts.Categories {
			for _, itemCat := range itemCats {
				if reqCat == itemCat {
					catMatch = true
					break
				}
			}
			if catMatch {
				break
			}
		}
		if !catMatch {
			return false
		}
	}

	var tv tvInfo
	if !isMovie {
		tv = resolveTV(item.Release)
	}

	// Filter by TVDB series (releases without a known TVDB ID are kept)
	if opts.TVDBID > 0 {
		if isMovie || (tv.TVDBID > 0 && tv.TVDBID != opts.TVDBID) {
			return false
		}
	}

	// Filter by season
	if opts.hasSeason() {
		if isMovie {
			return false
		}
		if tv.Season != opts.Season {
			return false
		}
	}

	// Filter by episode
	if opts.Episode > 0 {
		if isMovie {
			return false
		}
		epStr := tvEpisodes(item.Torrent, tv)
		if epStr != "" {
			if strings.Contains(epStr, "-") {
				parts := strings.SplitN(epStr, "-", 2)
				startEp, err1 := strconv.Atoi(strings.TrimPrefix(parts[0], "E"))
				endEp, err2 := strconv.Atoi(strings.TrimPrefix(parts[1], "E"))
				if err1 == nil && err2 == nil {
					if opts.Episode < startEp || opts.Episode > endEp {
						return false
					}
				}
			} else {
				epNum, err := strconv.Atoi(strings.TrimPrefix(epStr, "E"))
				if err == nil && epNum != opts.Episode {
					return false
				}
			}
		}
	}

	return true
}
