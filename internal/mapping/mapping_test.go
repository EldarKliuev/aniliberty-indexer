package mapping

import "testing"

const sampleList = `[
  {"type":"TV","mal_id":38000,"tvdb_id":348545,"season":{"tvdb":1,"tmdb":1}},
  {"type":"TV","mal_id":47778,"tvdb_id":348545,"season":{"tvdb":3,"tmdb":3}},
  {"type":"TV","mal_id":49926,"tvdb_id":348545,"season":{"tvdb":2,"tmdb":2}},
  {"type":"TV","mal_id":48583,"tvdb_id":267440,"season":{"tvdb":4,"tmdb":4},"episode_offset":{"tvdb":16,"tmdb":16}},
  {"type":"MOVIE","mal_id":59192,"tvdb_id":348545,"season":{"tvdb":0}},
  {"type":"TV","mal_id":111,"tvdb_id":999},
  {"type":"TV","mal_id":222},
  {"type":"TV","anidb_id":5,"tvdb_id":777,"season":{"tvdb":1}}
]`

func TestParseAndLookup(t *testing.T) {
	if err := LoadFromBytes([]byte(sampleList)); err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	defer Reset()

	tests := []struct {
		mal       int
		ok        bool
		tvdb      int
		season    int
		hasSeason bool
		offset    int
	}{
		{47778, true, 348545, 3, true, 0},
		{48583, true, 267440, 4, true, 16},
		{59192, true, 348545, 0, true, 0},
		{111, true, 999, 0, false, 0},
		{222, false, 0, 0, false, 0}, // no TVDB ID
		{0, false, 0, 0, false, 0},
		{123456, false, 0, 0, false, 0},
	}
	for _, tt := range tests {
		e, ok := Lookup(tt.mal)
		if ok != tt.ok {
			t.Errorf("Lookup(%d) ok = %v, want %v", tt.mal, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if e.TVDBID != tt.tvdb || e.Season != tt.season || e.HasSeason != tt.hasSeason || e.EpisodeOffset != tt.offset {
			t.Errorf("Lookup(%d) = %+v, want tvdb=%d season=%d hasSeason=%v offset=%d",
				tt.mal, e, tt.tvdb, tt.season, tt.hasSeason, tt.offset)
		}
	}

	if Size() != 6 {
		t.Errorf("Size() = %d, want 6", Size())
	}
}

func TestEntriesForTVDB(t *testing.T) {
	if err := LoadFromBytes([]byte(sampleList)); err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	defer Reset()

	all := EntriesForTVDB(348545, -1)
	if len(all) != 4 {
		t.Fatalf("EntriesForTVDB(any season) = %d entries, want 4", len(all))
	}
	if all[0].MALID != 38000 {
		t.Errorf("entries are not sorted by MAL ID: %+v", all)
	}

	s3 := EntriesForTVDB(348545, 3)
	if len(s3) != 1 || s3[0].MALID != 47778 {
		t.Errorf("EntriesForTVDB(season 3) = %+v, want MAL 47778", s3)
	}

	s0 := EntriesForTVDB(348545, 0)
	if len(s0) != 1 || s0[0].MALID != 59192 {
		t.Errorf("EntriesForTVDB(season 0) = %+v, want MAL 59192", s0)
	}

	// Entry without a season must not match a concrete season
	if got := EntriesForTVDB(999, 0); len(got) != 0 {
		t.Errorf("EntriesForTVDB(999, 0) = %+v, want none", got)
	}
}

func TestNotLoaded(t *testing.T) {
	Reset()
	if Loaded() {
		t.Fatal("Loaded() = true after Reset")
	}
	if _, ok := Lookup(47778); ok {
		t.Error("Lookup succeeded without a loaded list")
	}
	if got := EntriesForTVDB(348545, -1); got != nil {
		t.Errorf("EntriesForTVDB without a list = %+v, want nil", got)
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	if err := LoadFromBytes([]byte(`[]`)); err == nil {
		t.Error("expected an error for an empty list")
	}
	if err := LoadFromBytes([]byte(`not json`)); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}
