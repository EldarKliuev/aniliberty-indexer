package torznab

import (
	"aniliberty-indexer/internal/api"
	"aniliberty-indexer/internal/config"
	"aniliberty-indexer/internal/torrentfile"

	"errors"
	"reflect"
	"strings"
	"testing"
)

func aoNoHakoFiles() []torrentfile.File {
	return []torrentfile.File{
		{Index: 0, Path: "Ao_no_Hako_2nd_Season_[02].mkv", Length: 1666294552},
		{Index: 1, Path: "Ao_no_Hako_2nd_Season_[01].mkv", Length: 979334548},
		{Index: 2, Path: "Ao_no_Hako_2nd_Season_[03].mkv", Length: 982571891},
		{Index: 3, Path: "Ao_no_Hako_2nd_Season_[04].mkv", Length: 983509472},
		{Index: 4, Path: "Ao_no_Hako_2nd_Season_[05].mkv", Length: 996599702},
	}
}

func TestPickEpisode(t *testing.T) {
	got := pickEpisode(aoNoHakoFiles(), 3, 0)
	want := &api.EpisodePick{Episode: 3, Files: []int{2}, Size: 982571891}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// TVDB S02E15 is AniLiberty [03] with offset 12
	if got := pickEpisode(aoNoHakoFiles(), 15, 12); got == nil || !reflect.DeepEqual(got.Files, []int{2}) {
		t.Errorf("offset: got %+v", got)
	}

	// Video and subtitles of the same episode
	withSubs := append(aoNoHakoFiles(), torrentfile.File{Index: 5, Path: "Subs/Ao_no_Hako_[03].ass", Length: 10}, torrentfile.File{Index: 6, Path: "cover_[03].jpg", Length: 5})
	if got := pickEpisode(withSubs, 3, 0); got == nil || !reflect.DeepEqual(got.Files, []int{2, 5}) || got.Size != 982571901 {
		t.Errorf("subtitles: got %+v", got)
	}

	if got := pickEpisode(aoNoHakoFiles(), 9, 0); got != nil {
		t.Errorf("missing episode: expected nil, got %+v", got)
	}
	if got := pickEpisode(aoNoHakoFiles()[2:3], 3, 0); got != nil {
		t.Errorf("nothing to skip: expected nil, got %+v", got)
	}
}

func TestSplitEpisode(t *testing.T) {
	loadTestMapping(t)

	pack := tvItem(0, "Ao no Hako 2nd Season", "Синяя шкатулка 2", "Ao no Hako 2nd Season [1-5]")
	pack.Torrent.Hash = "packhash"
	single := tvItem(0, "Ao no Hako 2nd Season", "Синяя шкатулка 2", "Ao no Hako 2nd Season [3]")
	single.Torrent.Hash = "singlehash"
	broken := tvItem(0, "Ao no Hako 2nd Season", "Синяя шкатулка 2", "Ao no Hako 2nd Season [1-5]")
	broken.Torrent.Hash = "brokenhash"
	movie := api.CombinedItem{}
	movie.Release.Type.Value = "MOVIE"
	movie.Torrent.Hash = "moviehash"

	fetched := map[string]int{}
	fetch := func(hash string) ([]torrentfile.File, error) {
		fetched[hash]++
		if hash == "brokenhash" {
			return nil, errors.New("boom")
		}
		return aoNoHakoFiles(), nil
	}
	out := SplitEpisode([]api.CombinedItem{pack, single, broken, movie}, 3, fetch)

	if out[0].Episode == nil || !reflect.DeepEqual(out[0].Episode.Files, []int{2}) {
		t.Errorf("pack: got %+v", out[0].Episode)
	}
	for i := 1; i < 4; i++ {
		if out[i].Episode != nil {
			t.Errorf("item %d must stay as is, got %+v", i, out[i].Episode)
		}
	}
	if fetched["singlehash"] != 0 || fetched["moviehash"] != 0 {
		t.Errorf("single episodes and movies must not be fetched: %v", fetched)
	}
	if pack.Episode != nil {
		t.Errorf("input items must not be modified")
	}
}

func TestBuildTorznabXMLEpisodePick(t *testing.T) {
	item := tvItem(0, "Ao no Hako 2nd Season", "Синяя шкатулка 2", "Ao no Hako 2nd Season [1-5]")
	item.Torrent.Hash = "packhash"
	item.Torrent.Size = 5608310165
	item.Torrent.Magnet = "magnet:?xt=urn:btih:packhash"
	item.Episode = &api.EpisodePick{Episode: 3, Files: []int{2}, Size: 982571891}

	origTpl := config.TORRENT_TITLE_TEMPLATE
	config.TORRENT_TITLE_TEMPLATE = "{title_latin_clean} - {season_episodes} [{quality}]"
	defer func() { config.TORRENT_TITLE_TEMPLATE = origTpl }()

	xml := BuildTorznabXML([]api.CombinedItem{item}, "http://indexer:3649")
	for _, want := range []string{
		`<title>Ao no Hako - S02E03 [1080p]</title>`,
		`<guid isPermaLink="false">packhash-E03</guid>`,
		`<size>982571891</size>`,
		`url="http://indexer:3649/download/packhash?files=2"`,
		`<torznab:attr name="episode" value="3" />`,
		`<torznab:attr name="infohash" value="packhash" />`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("expected %q in:\n%s", want, xml)
		}
	}
	if strings.Contains(xml, "magneturl") {
		t.Errorf("single-episode item must not carry the pack magnet")
	}

	// Without a base URL the pick cannot be served and the pack is announced as is
	plain := BuildTorznabXML([]api.CombinedItem{item}, "")
	if !strings.Contains(plain, "<size>5608310165</size>") || strings.Contains(plain, "packhash-E03") {
		t.Errorf("expected the whole pack without base URL:\n%s", plain)
	}
}

func TestParseFileIndices(t *testing.T) {
	if got := ParseFileIndices("2, 5,x,-1"); !reflect.DeepEqual(got, []int{2, 5}) {
		t.Errorf("got %v", got)
	}
	if got := FormatFileIndices([]int{0, 3}); got != "0,3" {
		t.Errorf("got %q", got)
	}
}

func TestSingleEpisodeTitleWithSeasonOnlyTemplate(t *testing.T) {
	origTpl := config.TORRENT_TITLE_TEMPLATE
	defer func() { config.TORRENT_TITLE_TEMPLATE = origTpl }()
	config.TORRENT_TITLE_TEMPLATE = "[AniLiberty] {title_latin_clean} - S{season} [RUS][{type} {quality} {codec}] ({year})"

	item := tvItem(0, "Ao no Hako 2nd Season", "Синяя шкатулка 2", "Ao no Hako 2nd Season [1-5]")
	item.Release.Year = 2026
	item.Torrent.Hash = "packhash"
	item.Torrent.Type.Value = "WEB-DL"

	// Season pack keeps the season-only name
	if got := BuildTVTitle(item.Release, item.Torrent); got != "[AniLiberty] Ao no Hako - S2 [RUS][WEB-DL 1080p x264] (2026)" {
		t.Errorf("pack: got %q", got)
	}

	// Episode picked out of the pack
	item.Episode = &api.EpisodePick{Episode: 5, Files: []int{4}, Size: 1}
	xml := BuildTorznabXML([]api.CombinedItem{item}, "http://indexer:3649")
	if want := "<title>[AniLiberty] Ao no Hako - S02E05 [RUS][WEB-DL 1080p x264] (2026)</title>"; !strings.Contains(xml, want) {
		t.Errorf("expected %q in:\n%s", want, xml)
	}

	// Torrent with a single episode
	item.Episode = nil
	item.Torrent.Label = "Ao no Hako 2nd Season [5]"
	if got := BuildTVTitle(item.Release, item.Torrent); got != "[AniLiberty] Ao no Hako - S02E05 [RUS][WEB-DL 1080p x264] (2026)" {
		t.Errorf("single-episode torrent: got %q", got)
	}
}

func TestSingleEpisodeTemplate(t *testing.T) {
	tests := map[string]string{
		"{title_latin_clean} - S{season} [{quality}]":         "{title_latin_clean} - {season_episodes} [{quality}]",
		"{title_latin_clean} - {season_episodes} [{quality}]": "{title_latin_clean} - {season_episodes} [{quality}]",
		"{title_latin_clean} {episodes}":                      "{title_latin_clean} {episodes}",
		"{title_latin_clean} [{quality}]":                     "{title_latin_clean} [{quality}] {season_episodes}",
	}
	for in, want := range tests {
		if got := singleEpisodeTemplate(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
