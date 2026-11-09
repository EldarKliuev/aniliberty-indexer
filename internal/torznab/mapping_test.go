package torznab

import (
	"aniliberty-indexer/internal/api"
	"aniliberty-indexer/internal/config"
	"aniliberty-indexer/internal/mapping"

	"strings"
	"testing"
)

const testMappingList = `[
  {"mal_id":38000,"tvdb_id":348545,"season":{"tvdb":1}},
  {"mal_id":47778,"tvdb_id":348545,"season":{"tvdb":3}},
  {"mal_id":64503,"tvdb_id":424019,"season":{"tvdb":2},"episode_offset":{"tvdb":12}},
  {"mal_id":777,"tvdb_id":555}
]`

func loadTestMapping(t *testing.T) {
	t.Helper()
	if err := mapping.LoadFromBytes([]byte(testMappingList)); err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	t.Cleanup(mapping.Reset)
}

func tvItem(mal int, latin, ru, label string) api.CombinedItem {
	var r api.Release
	r.Name.Latin = latin
	r.Name.Main = ru
	r.Type.Value = "TV"
	r.Mal.ID = mal
	var tr api.Torrent
	tr.Label = label
	tr.Type.Value = "WEBRip"
	tr.Quality.Value = "1080p"
	tr.Codec.Value = "AVC"
	return api.CombinedItem{Release: r, Torrent: tr}
}

func TestResolveTVUsesMapping(t *testing.T) {
	loadTestMapping(t)

	tests := []struct {
		name   string
		item   api.CombinedItem
		season int
		tvdb   int
		mapped bool
	}{
		{"season from mapping, no number in title", tvItem(47778, "Kimetsu no Yaiba: Yuukaku-hen", "Клинок", ""), 3, 348545, true},
		{"mapping beats title", tvItem(64503, "Yozakura-san Chi no Daisakusen 2nd Season Part 2", "", ""), 2, 424019, true},
		{"known series without season falls back to title", tvItem(777, "Some Show 3rd Season", "", ""), 3, 555, false},
		{"unknown MAL ID falls back to title", tvItem(1, "Other Show Season 2", "", ""), 2, 0, false},
		{"shikimori ID is used when MAL is missing", func() api.CombinedItem {
			it := tvItem(0, "Kimetsu no Yaiba: Yuukaku-hen", "", "")
			it.Release.Shikimori.ID = 47778
			return it
		}(), 3, 348545, true},
	}
	for _, tt := range tests {
		info := resolveTV(tt.item.Release)
		if info.Season != tt.season || info.TVDBID != tt.tvdb || info.Mapped != tt.mapped {
			t.Errorf("%s: got %+v, want season=%d tvdb=%d mapped=%v", tt.name, info, tt.season, tt.tvdb, tt.mapped)
		}
	}
}

func TestShiftEpisodes(t *testing.T) {
	tests := []struct {
		in     string
		offset int
		want   string
	}{
		{"E01-E04", 12, "E13-E16"},
		{"E05", 16, "E21"},
		{"E01-E12", 0, "E01-E12"},
		{"", 12, ""},
		{"E99-E100", 1, "E100-E101"},
	}
	for _, tt := range tests {
		if got := shiftEpisodes(tt.in, tt.offset); got != tt.want {
			t.Errorf("shiftEpisodes(%q, %d) = %q, want %q", tt.in, tt.offset, got, tt.want)
		}
	}
}

func TestBuildTVTitleWithMapping(t *testing.T) {
	loadTestMapping(t)
	orig := config.TORRENT_TITLE_TEMPLATE
	defer func() { config.TORRENT_TITLE_TEMPLATE = orig }()
	config.TORRENT_TITLE_TEMPLATE = "{title_latin_clean} - {season_episodes}"

	it := tvItem(64503, "Yozakura-san Chi no Daisakusen 2nd Season Part 2", "", "Yozakura - AniLiberty.TOP [WEB-DL 1080p][AVC][1-4]")
	got := BuildTVTitle(it.Release, it.Torrent)
	want := "Yozakura-san Chi no Daisakusen Part 2 - S02E13-S02E16"
	if got != want {
		t.Errorf("BuildTVTitle = %q, want %q", got, want)
	}

	it = tvItem(47778, "Kimetsu no Yaiba: Yuukaku-hen", "", "[WEBRip 1080p][AVC][1-11]")
	got = BuildTVTitle(it.Release, it.Torrent)
	want = "Kimetsu no Yaiba: Yuukaku-hen - S03E01-S03E11"
	if got != want {
		t.Errorf("BuildTVTitle = %q, want %q", got, want)
	}
}

func TestBuildTorznabXMLMappingAttrs(t *testing.T) {
	loadTestMapping(t)

	it := tvItem(64503, "Yozakura-san Chi no Daisakusen 2nd Season Part 2", "", "[WEB-DL 1080p][AVC][1-4]")
	xml := BuildTorznabXML([]api.CombinedItem{it}, "http://localhost:3649")
	for _, want := range []string{
		`<torznab:attr name="tvdbid" value="424019" />`,
		`<torznab:attr name="season" value="2" />`,
		`<torznab:attr name="episode" value="13-16" />`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("XML does not contain %s:\n%s", want, xml)
		}
	}

	unmapped := tvItem(0, "Other Show", "", "[WEBRip 1080p][1-12]")
	xml = BuildTorznabXML([]api.CombinedItem{unmapped}, "http://localhost:3649")
	if strings.Contains(xml, `name="tvdbid"`) {
		t.Errorf("unmapped release must not have a tvdbid attr:\n%s", xml)
	}
}

func TestFilterItemsWithMapping(t *testing.T) {
	loadTestMapping(t)

	s1 := tvItem(38000, "Kimetsu no Yaiba", "", "[WEBRip 1080p][1-26]")
	s3 := tvItem(47778, "Kimetsu no Yaiba: Yuukaku-hen", "", "[WEBRip 1080p][1-11]")
	other := tvItem(64503, "Yozakura-san Chi no Daisakusen 2nd Season Part 2", "", "[WEB-DL 1080p][1-4]")
	unmapped := tvItem(0, "Kimetsu no Yaiba Recap", "", "[WEBRip 1080p][1]")
	all := []api.CombinedItem{s1, s3, other, unmapped}

	got := FilterItems(all, FilterOptions{Season: 3})
	if len(got) != 1 || got[0].Release.Mal.ID != 47778 {
		t.Errorf("season 3 filter: got %d items, want only Yuukaku-hen", len(got))
	}

	got = FilterItems(all, FilterOptions{TVDBID: 348545})
	if len(got) != 3 {
		t.Errorf("tvdbid filter: got %d items, want 3 (two mapped + one unmapped)", len(got))
	}

	// Episode filter must use TVDB numbering (1-4 shifted by 12 -> 13-16)
	if got := FilterItems([]api.CombinedItem{other}, FilterOptions{Season: 2, Episode: 14}); len(got) != 1 {
		t.Errorf("episode 14 must match shifted batch 13-16, got %d", len(got))
	}
	if got := FilterItems([]api.CombinedItem{other}, FilterOptions{Season: 2, Episode: 2}); len(got) != 0 {
		t.Errorf("episode 2 must not match shifted batch 13-16, got %d", len(got))
	}

	// Explicit season 0 (specials) filters, zero value does not
	if got := FilterItems(all, FilterOptions{SeasonSet: true, Season: 0}); len(got) != 0 {
		t.Errorf("season 0 filter: got %d items, want 0", len(got))
	}
	if got := FilterItems(all, FilterOptions{}); len(got) != 4 {
		t.Errorf("empty filter: got %d items, want 4", len(got))
	}
}
