package server

import (
	"aniliberty-indexer/internal/api"
	"aniliberty-indexer/internal/config"
	"aniliberty-indexer/internal/mapping"
	"aniliberty-indexer/internal/qbit"
	"aniliberty-indexer/internal/torznab"

	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Main HTTP handler: /, /health, /download/, /api, /torznab
func HandleRequest(w http.ResponseWriter, req *http.Request) {
	parsedUrl := req.URL
	pathname := parsedUrl.Path
	query := parsedUrl.Query()

	// Health check
	if pathname == "/" || pathname == "/health" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","name":"AniLiberty Indexer"}`))
		return
	}

	// Validate the API key (unrelated to AniLiberty)
	if config.API_KEY != "" {
		if !checkAPIKey(req, query) {
			respondUnauthorized(w, pathname)
			return
		}
	}

	// Relay a .torrent file from AniLiberty API to the client
	if strings.HasPrefix(pathname, "/download/") {
		hashOrId := strings.TrimPrefix(pathname, "/download/")
		// Single-episode item: only these files of the pack are to be downloaded
		if files := query.Get("files"); files != "" {
			qbit.SelectFiles(hashOrId, torznab.ParseFileIndices(files))
		}
		pk := query.Get("pk")
		downloadUrl := fmt.Sprintf("%s/anime/torrents/%s/file", config.ANILIBERTY_API, hashOrId)
		if pk != "" {
			downloadUrl += "?pk=" + url.QueryEscape(pk)
		}
		api.RelayTorrentFile(downloadUrl, w)
		return
	}

	// Torznab API
	if pathname == "/api" || pathname == "/torznab" || pathname == "/torznab/api" {
		handleTorznab(w, req, query)
		return
	}

	http.Error(w, "Not Found", http.StatusNotFound)
}

// Checks the service API key from query, X-Api-Key header, or Authorization Bearer
func checkAPIKey(req *http.Request, query url.Values) bool {
	reqKey := query.Get("apikey")
	if reqKey == "" {
		reqKey = req.Header.Get("X-Api-Key")
	}
	if reqKey == "" {
		authHeader := req.Header.Get("Authorization")
		if authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				reqKey = authHeader[7:]
			} else {
				reqKey = authHeader
			}
		}
	}
	return reqKey == config.API_KEY
}

// Replies 401 (XML for Torznab paths, JSON for everything else)
func respondUnauthorized(w http.ResponseWriter, pathname string) {
	if strings.HasPrefix(pathname, "/api") || strings.HasPrefix(pathname, "/torznab") {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<error code="100" description="Invalid API Key" />`))
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"Unauthorized: Invalid or missing API key"}`))
	}
}

// Routes Torznab requests by the t= parameter: caps, search, tvsearch, movie, or browse
func handleTorznab(w http.ResponseWriter, req *http.Request, query url.Values) {
	t := strings.ToLower(query.Get("t"))

	if t == "caps" {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(torznab.BuildCapsXML()))
		return
	}

	if t == "" || t == "browse" || t == "search" || t == "tvsearch" || t == "tv" || t == "movie" || t == "movie-search" || t == "moviesearch" {
		handleSearch(w, req, query)
		return
	}

	errMsg := "Unknown Torznab action"
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<error code="201" description="` + errMsg + `" />`))
}

// Parses comma-separated category IDs from cat query parameters
func parseCategories(query url.Values) []int {
	var cats []int
	for _, val := range query["cat"] {
		for _, part := range strings.Split(val, ",") {
			part = strings.TrimSpace(part)
			if c, err := strconv.Atoi(part); err == nil {
				cats = append(cats, c)
			}
		}
	}
	return cats
}

var yearSuffixRegex = regexp.MustCompile(`\s+\d{4}$`)

// Handles search (q= set) or browse (q= empty) and returns a Torznab XML feed
func handleSearch(w http.ResponseWriter, req *http.Request, query url.Values) {
	q := query.Get("q")
	t := strings.ToLower(query.Get("t"))

	if q != "" {
		q = yearSuffixRegex.ReplaceAllString(q, "")
	}

	limit := config.DEFAULT_LIMIT
	if l, err := strconv.Atoi(query.Get("limit")); err == nil {
		limit = l
	}
	if limit > config.MAX_LIMIT {
		limit = config.MAX_LIMIT
	}
	offset := 0
	if o, err := strconv.Atoi(query.Get("offset")); err == nil {
		offset = o
	}

	cats := parseCategories(query)
	if len(cats) == 0 && (t == "movie" || t == "movie-search" || t == "moviesearch") {
		cats = []int{2000, 2030, 2040, 2045, 2050, 2070}
	}

	season, seasonSet := 0, false
	if s, err := strconv.Atoi(query.Get("season")); err == nil && s >= 0 {
		season, seasonSet = s, true
	}
	var ep int
	if e, err := strconv.Atoi(query.Get("ep")); err == nil {
		ep = e
	}
	var tvdbID int
	if id, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(query.Get("tvdbid")), "tvdb")); err == nil && id > 0 {
		tvdbID = id
	}

	hasFilter := len(cats) > 0 || seasonSet || ep > 0 || tvdbID > 0

	var items []api.CombinedItem
	var err error
	idSearchDone := false

	// Search by TVDB ID: TVDB series -> MAL entries -> AniLiberty releases
	if tvdbID > 0 && mapping.Loaded() {
		seasonFilter := -1
		if seasonSet {
			seasonFilter = season
		}
		var malIDs []int
		for _, e := range mapping.EntriesForTVDB(tvdbID, seasonFilter) {
			malIDs = append(malIDs, e.MALID)
		}
		releaseIDs := api.ReleaseIDsForMAL(malIDs)
		log.Printf("[search] tvdbid=%d season=%s ep=%s mal=%v releases=%v\n", tvdbID, query.Get("season"), query.Get("ep"), malIDs, releaseIDs)
		if len(releaseIDs) > 0 {
			items = api.FetchReleasesByIDs(releaseIDs, 0)
			idSearchDone = true
		} else if q == "" && api.CatalogReady() {
			// Nothing on AniLiberty for this series/season
			idSearchDone = true
		}
	}

	if idSearchDone {
		// items already set
	} else if tvdbID > 0 && q == "" {
		// Unknown TVDB ID and no text query: never fall back to the unfiltered latest feed
		log.Printf("[search] tvdbid=%d not resolved, returning empty result\n", tvdbID)
		items = []api.CombinedItem{}
	} else if q != "" {
		searchLimit := limit
		if hasFilter && searchLimit < config.MAX_LIMIT*2 {
			searchLimit = config.MAX_LIMIT * 2
		}
		log.Printf("[search] query=\"%s\" season=%s ep=%s limit=%d\n", q, query.Get("season"), query.Get("ep"), searchLimit)
		items, err = api.SearchTorrents(q, searchLimit)
	} else {
		page := 1
		if limit > 0 {
			page = (offset / limit) + 1
		}
		if page < 1 {
			page = 1
		}
		log.Printf("[browse] limit=%d offset=%d page=%d\n", limit, offset, page)
		items, err = api.FetchLatestTorrents(limit, page)
	}

	if err != nil {
		log.Printf("[error] %v\n", err)
		http.Error(w, fmt.Sprintf("Internal Server Error: %v", err), http.StatusInternalServerError)
		return
	}

	if hasFilter {
		items = torznab.FilterItems(items, torznab.FilterOptions{
			Categories: cats,
			Season:     season,
			SeasonSet:  seasonSet,
			Episode:    ep,
			TVDBID:     tvdbID,
		})
	}

	if len(items) > limit {
		items = items[:limit]
	}

	// Episode search: announce packs as the requested episode only. With QBIT_URL set the
	// other files are skipped after the grab; without it the whole pack is downloaded and
	// Sonarr imports just the requested episode.
	if ep > 0 {
		items = torznab.SplitEpisode(items, ep, api.FetchTorrentFiles)
	}

	baseURL := config.BASE_URL
	if baseURL == "" && req != nil {
		scheme := "http"
		if req.TLS != nil || req.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := req.Host
		if host == "" {
			host = config.HOST + ":" + config.PORT
		}
		baseURL = scheme + "://" + host
	}

	xmlResponse := torznab.BuildTorznabXML(items, baseURL)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(xmlResponse))
}
