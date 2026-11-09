package api

// Anime release entry from AniLiberty API
type Release struct {
	ID    int    `json:"id"`
	Alias string `json:"alias"`
	Name  struct {
		Main  string `json:"main"`
		Latin string `json:"english"`
	} `json:"name"`
	Year        int    `json:"year"`
	Description string `json:"description"`
	Type        struct {
		Value string `json:"value"`
	} `json:"type"`
	Mal struct {
		ID int `json:"id"`
	} `json:"mal"`
	Shikimori struct {
		ID int `json:"id"`
	} `json:"shikimori"`
	Poster struct {
		Optimized struct {
			Src string `json:"src"`
		} `json:"optimized"`
		Src string `json:"src"`
	} `json:"poster"`
}

// Returns the MyAnimeList ID of the release (Shikimori shares MAL IDs)
func (r Release) MALID() int {
	if r.Mal.ID > 0 {
		return r.Mal.ID
	}
	return r.Shikimori.ID
}

// Release with its torrents, as returned by /anime/releases/{id}
type ReleaseWithTorrents struct {
	Release
	Torrents []Torrent `json:"torrents"`
}

// /anime/catalog/releases page with only id and MAL fields included
type catalogPage struct {
	Data []struct {
		ID  int `json:"id"`
		Mal struct {
			ID int `json:"id"`
		} `json:"mal"`
		Shikimori struct {
			ID int `json:"id"`
		} `json:"shikimori"`
	} `json:"data"`
	Meta struct {
		Pagination struct {
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	} `json:"meta"`
}

// Torrent file associated with a release
type Torrent struct {
	ID   int    `json:"id"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Type struct {
		Value string `json:"value"`
	} `json:"type"`
	Codec struct {
		Value string `json:"value"`
	} `json:"codec"`
	Label   string `json:"label"`
	Quality struct {
		Value string `json:"value"`
	} `json:"quality"`
	Magnet         string `json:"magnet"`
	Seeders        int    `json:"seeders"`
	Leechers       int    `json:"leechers"`
	Description    string `json:"description"`
	UpdatedAt      string `json:"updated_at"`
	CompletedTimes int    `json:"completed_times"`
}

// Pairs a release with one of its torrents
type CombinedItem struct {
	Release Release
	Torrent Torrent
}

// /anime/torrents API response
type LatestTorrentsResponse struct {
	Data []struct {
		Release Release `json:"release"`
		Torrent
	} `json:"data"`
}
