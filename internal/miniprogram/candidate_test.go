package miniprogram

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseContentRangeValidatesCompleteEntityLength(t *testing.T) {
	for _, raw := range []string{"bytes 0-2/*", "bytes 3-2/6", "bytes 0-6/6", "bytes -1-2/6", "bytes 0-2/9999999999999999999999999", "garbage"} {
		if _, _, _, ok := ParseContentRange(raw); ok {
			t.Fatalf("accepted invalid range %q", raw)
		}
	}
	start, end, total, ok := ParseContentRange("bytes 3-5/6")
	if !ok || start != 3 || end != 5 || total != 6 {
		t.Fatal("valid range rejected")
	}
}

func TestClassifyDetectsMedia(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		rawURL      string
		wantKind    string
		wantSuffix  string
	}{
		{name: "jpeg content type", contentType: "image/jpeg", rawURL: "https://example.com/img", wantKind: "image", wantSuffix: ".jpg"},
		{name: "webp extension fallback", contentType: "application/octet-stream", rawURL: "https://example.com/cover.webp?token=1", wantKind: "image", wantSuffix: ".webp"},
		{name: "mp4 content type", contentType: "video/mp4; charset=utf-8", rawURL: "https://example.com/play", wantKind: "video", wantSuffix: ".mp4"},
		{name: "m3u8 content type", contentType: "application/vnd.apple.mpegurl", rawURL: "https://example.com/play", wantKind: "m3u8", wantSuffix: ".m3u8"},
		{name: "mp4 extension fallback", contentType: "application/octet-stream", rawURL: "https://example.com/video.mp4?token=1", wantKind: "video", wantSuffix: ".mp4"},
		{name: "mp3 content type without extension", contentType: "audio/mpeg", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".mp3"},
		{name: "audio mp4 before video extension", contentType: "audio/mp4; codecs=mp4a.40.2", rawURL: "https://example.com/audio.mp4", wantKind: "audio", wantSuffix: ".m4a"},
		{name: "aac content type", contentType: "audio/aac", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".aac"},
		{name: "wav content type", contentType: "audio/vnd.wave", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".wav"},
		{name: "ogg content type", contentType: "audio/ogg", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".ogg"},
		{name: "flac content type", contentType: "audio/flac", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".flac"},
		{name: "opus content type", contentType: "audio/opus", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".opus"},
		{name: "amr content type", contentType: "audio/amr", rawURL: "https://example.com/play", wantKind: "audio", wantSuffix: ".amr"},
		{name: "video ogg is not audio", contentType: "video/ogg", rawURL: "https://example.com/clip.ogg", wantKind: "", wantSuffix: ""},
		{name: "unknown", contentType: "text/html", rawURL: "https://example.com/page", wantKind: "", wantSuffix: ""},
	}
	for _, extension := range []string{"mp3", "m4a", "aac", "wav", "ogg", "flac", "opus", "amr"} {
		tests = append(tests, struct {
			name, contentType, rawURL, wantKind, wantSuffix string
		}{extension + " extension", "application/octet-stream", "https://example.com/track." + extension + "?token=1", "audio", "." + extension})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotSuffix := Classify(tt.contentType, tt.rawURL)
			if gotKind != tt.wantKind || gotSuffix != tt.wantSuffix {
				t.Fatalf("Classify() = (%q, %q), want (%q, %q)", gotKind, gotSuffix, tt.wantKind, tt.wantSuffix)
			}
		})
	}
}

func TestExtractMediaURLsFromJSON(t *testing.T) {
	body := []byte(`{
		"data": {
			"videoUrl": "https://cdn.example.com/a.mp4?token=1",
			"nested": {"playUrl": "https://cdn.example.com/live/index.m3u8"},
			"coverImage": "https://cdn.example.com/cover.jpg",
			"ignore": "https://cdn.example.com/page.html"
		}
	}`)
	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 3 {
		t.Fatalf("len(ExtractMediaURLsFromJSON()) = %d, want 3: %#v", len(got), got)
	}
	if got[0].Kind == "" || got[1].Kind == "" {
		t.Fatalf("expected classified media candidates, got %#v", got)
	}
}

func TestExtractMediaURLsFromJSONFindsImageFields(t *testing.T) {
	body := []byte(`{
		"data": {
			"avatar": "https://cdn.example.com/avatar.png",
			"poster": "https://cdn.example.com/poster.avif",
			"thumbnailUrl": "https://cdn.example.com/thumb.webp"
		}
	}`)
	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 3 {
		t.Fatalf("len(ExtractMediaURLsFromJSON()) = %d, want 3: %#v", len(got), got)
	}
	for _, item := range got {
		if item.Kind != "image" {
			t.Fatalf("expected image candidate, got %#v", item)
		}
	}
}

func TestExtractMediaURLsFromJSONUsesNearbyTitle(t *testing.T) {
	body := []byte(`{
		"data": {
			"title": "春日活动",
			"videoUrl": "https://cdn.example.com/videos/spring.mp4"
		}
	}`)

	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 1 {
		t.Fatalf("len(ExtractMediaURLsFromJSON()) = %d, want 1: %#v", len(got), got)
	}
	if got[0].Title != "春日活动" {
		t.Fatalf("Title = %q, want nearby JSON title", got[0].Title)
	}
}

func TestExtractMediaURLsFromJSONUsesAudioTitle(t *testing.T) {
	got := ExtractMediaURLsFromJSON([]byte(`{"audioTitle":"课程讲解","voiceUrl":"https://cdn.example.com/lesson.m4a"}`))
	if len(got) != 1 || got[0].Kind != "audio" || got[0].Suffix != ".m4a" || got[0].Title != "课程讲解" || got[0].FieldPath != "voiceUrl" {
		t.Fatalf("JSON audio title or source was lost: %#v", got)
	}
}

func TestCleanHeadersRemovesUnsafeReplayHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("User-Agent", "ua")
	h.Set("Referer", "https://example.com")
	h.Set("Cookie", "a=b")
	h.Set("Proxy-Authorization", "proxy-secret")
	h.Set("Host", "bad")
	h.Set("Accept-Encoding", "gzip")
	h.Set("If-None-Match", "etag")
	h.Set("If-Modified-Since", "yesterday")
	h.Set("Range", "bytes=0-")
	got := CleanHeaders(h)
	if got["User-Agent"] != "ua" || got["Referer"] != "https://example.com" || got["Cookie"] != "a=b" {
		t.Fatalf("expected replay headers to be preserved, got %#v", got)
	}
	if _, ok := got["Host"]; ok {
		t.Fatalf("Host should be removed, got %#v", got)
	}
	if _, ok := got["Proxy-Authorization"]; ok {
		t.Fatal("proxy credentials must not be replayed to media servers")
	}
	if _, ok := got["Accept-Encoding"]; ok {
		t.Fatalf("Accept-Encoding should be removed, got %#v", got)
	}
	if _, ok := got["If-None-Match"]; ok {
		t.Fatalf("If-None-Match should be removed, got %#v", got)
	}
	if _, ok := got["If-Modified-Since"]; ok {
		t.Fatalf("If-Modified-Since should be removed, got %#v", got)
	}
	if _, ok := got["Range"]; ok {
		t.Fatalf("Range should be removed, got %#v", got)
	}
}

func TestStoreOnCandidateAddedRunsAfterUnlock(t *testing.T) {
	store := NewStore()
	done := make(chan struct{})
	store.OnCandidateAdded = func(Candidate) {
		_ = store.List("")
		close(done)
	}

	go store.Add(Candidate{URL: "https://example.com/video.mp4", AppID: "app"})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("OnCandidateAdded appears to run while store lock is held")
	}
}

func TestStoreAddMergesDuplicateM3U8Metadata(t *testing.T) {
	store := NewStore()
	first, changed := store.Add(Candidate{
		AppID: "app",
		URL:   "https://example.com/index.m3u8",
		Kind:  "m3u8",
	})
	if !changed {
		t.Fatal("first Add should report a change")
	}

	updated, changed := store.Add(Candidate{
		AppID:         "app",
		URL:           first.URL,
		Kind:          "m3u8",
		ContentType:   "application/vnd.apple.mpegurl",
		ContentLength: 1234,
		Headers:       map[string]string{"Referer": "https://example.com"},
		CachedPath:    "downloads/index.m3u8",
	})
	if !changed {
		t.Fatal("duplicate Add with cache metadata should report a change")
	}
	if updated.CachedPath != "downloads/index.m3u8" {
		t.Fatalf("CachedPath = %q, want updated path", updated.CachedPath)
	}

	list := store.List("app")
	if len(list) != 1 {
		t.Fatalf("len(List) = %d, want 1", len(list))
	}
	if list[0].CachedPath != "downloads/index.m3u8" || list[0].ContentLength != 1234 {
		t.Fatalf("stored candidate was not merged: %#v", list[0])
	}
	if list[0].Headers["Referer"] != "https://example.com" {
		t.Fatalf("stored headers were not merged: %#v", list[0].Headers)
	}
}

func TestStoreUnattributedBucketIsIndependentAndDeduplicated(t *testing.T) {
	store := NewStore()
	store.SetTarget(Target{AppID: "wx123"})
	_, generation := store.Observe("wx123")
	if _, ok := store.AddForTarget(generation, Candidate{AppID: "wx123", URL: "https://cdn.example.com/target.mp4", Kind: "video"}, nil); !ok {
		t.Fatal("target candidate should be admitted")
	}
	first, ok := store.AddUnattributed(Candidate{
		URL:     "https://h5.example.com/lesson.mp4?sign=1",
		Kind:    "video",
		Suffix:  ".mp4",
		Headers: map[string]string{"Referer": "https://h5.example.com/course", "Cookie": "sid=old"},
	})
	if !ok || first.Attribution != AttributionUnattributed {
		t.Fatalf("AddUnattributed() = %#v, %v", first, ok)
	}
	if got := store.List(""); len(got) != 1 || got[0].AppID != "wx123" {
		t.Fatalf("target list = %#v, want only the confirmed candidate", got)
	}
	unattributed := store.Unattributed()
	if len(unattributed) != 1 || unattributed[0].URL != first.URL || unattributed[0].AppID != "" {
		t.Fatalf("unattributed list = %#v", unattributed)
	}

	updated, changed := store.AddUnattributed(Candidate{
		URL:         first.URL,
		Kind:        "video",
		Suffix:      ".mp4",
		SourceAppID: "wx456",
		Headers:     map[string]string{"Referer": "https://h5.example.com/course", "Cookie": "sid=fresh"},
	})
	if !changed || updated.Headers["Cookie"] != "sid=fresh" {
		t.Fatalf("duplicate unattributed candidate did not refresh headers: %#v", updated)
	}
	if got := store.Unattributed(); len(got) != 1 {
		t.Fatalf("duplicate unattributed candidate was not merged: %#v", got)
	}
	if got := store.Unattributed()[0]; got.SourceAppID != "wx456" {
		t.Fatalf("duplicate merge should refresh source AppID, got %q", got.SourceAppID)
	}
	store.ClearUnattributed()
	if got := store.Unattributed(); len(got) != 0 {
		t.Fatalf("ClearUnattributed() kept candidates: %#v", got)
	}
}

func TestStoreUnattributedBucketDropsOldestBeyondLimit(t *testing.T) {
	store := NewStore()
	for i := 0; i < unattributedLimit+3; i++ {
		store.AddUnattributed(Candidate{URL: "https://h5.example.com/media/" + strconv.Itoa(i) + ".mp4", Kind: "video"})
	}
	got := store.Unattributed()
	if len(got) != unattributedLimit {
		t.Fatalf("unattributed len = %d, want %d", len(got), unattributedLimit)
	}
	if !strings.HasSuffix(got[0].URL, "/3.mp4") || !strings.HasSuffix(got[len(got)-1].URL, "/502.mp4") {
		t.Fatalf("unattributed window = %q ... %q", got[0].URL, got[len(got)-1].URL)
	}
}

func TestStoreClearListClearsUnattributedWhileTargetSwitchKeepsIt(t *testing.T) {
	store := NewStore()
	store.SetTarget(Target{AppID: "wx123"})
	_, generation := store.Observe("wx123")
	store.AddForTarget(generation, Candidate{AppID: "wx123", URL: "https://cdn.example.com/target.mp4", Kind: "video"}, nil)
	store.AddUnattributed(Candidate{URL: "https://h5.example.com/lesson.mp4", Kind: "video"})

	store.SetTarget(Target{AppID: "wx456"})
	if got := store.List(""); len(got) != 0 {
		t.Fatalf("target switch kept target candidates: %#v", got)
	}
	if got := store.Unattributed(); len(got) != 1 {
		t.Fatalf("target switch should keep unattributed candidates, got %#v", got)
	}

	store.Clear("wx456")
	if got := store.Unattributed(); len(got) != 0 {
		t.Fatalf("clear list should clear unattributed candidates, got %#v", got)
	}

	store.AddUnattributed(Candidate{URL: "https://h5.example.com/lesson2.mp4", Kind: "video"})
	store.Clear("")
	if got := store.Unattributed(); len(got) != 0 {
		t.Fatalf("clear all should clear unattributed candidates, got %#v", got)
	}
}

func TestExtractMediaURLsFromJSONJoinsAdjacentToken(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantURL string
	}{
		{
			name:    "question mark token",
			body:    `{"url":"https://cdn.example.com/v.mp4","urlToken":"?auth=1"}`,
			wantURL: "https://cdn.example.com/v.mp4?auth=1",
		},
		{
			name:    "ampersand token with existing query",
			body:    `{"playUrl":"https://cdn.example.com/v.mp4?a=1","playToken":"&token=2"}`,
			wantURL: "https://cdn.example.com/v.mp4?a=1&token=2",
		},
		{
			name:    "ampersand token without query",
			body:    `{"videoUrl":"https://cdn.example.com/v.mp4","authKey":"&k=3"}`,
			wantURL: "https://cdn.example.com/v.mp4?k=3",
		},
		{
			name:    "nested pair",
			body:    `{"data":{"mediaUrl":"https://cdn.example.com/v.mp4","mediaUrlToken":"?sig=4"}}`,
			wantURL: "https://cdn.example.com/v.mp4?sig=4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMediaURLsFromJSON([]byte(tt.body))
			if len(got) != 1 {
				t.Fatalf("candidates = %#v, want exactly the joined URL", got)
			}
			if got[0].URL != tt.wantURL || got[0].Kind != "video" || got[0].Suffix != ".mp4" {
				t.Fatalf("candidate = %#v, want %q", got[0], tt.wantURL)
			}
		})
	}
}

func TestExtractMediaURLsFromJSONProvisionalStrongFields(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantKind   string
		wantSuffix string
	}{
		{name: "video field without suffix", body: `{"videoAddress":"https://cdn.example.com/play/abc123"}`, wantKind: "video", wantSuffix: ".mp4"},
		{name: "stream field without suffix", body: `{"streamUrl":"https://live.example.com/channel/9"}`, wantKind: "m3u8", wantSuffix: ".m3u8"},
		{name: "play field without suffix", body: `{"mediaPlay":"https://cdn.example.com/play/x"}`, wantKind: "video", wantSuffix: ".mp4"},
		{name: "cover field without suffix skipped", body: `{"cover":"https://cdn.example.com/img/abc"}`},
		{name: "weak field without suffix skipped", body: `{"homepage":"https://example.com/about"}`},
		{name: "segment still excluded", body: `{"videoUrl":"https://cdn.example.com/a/1.ts"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMediaURLsFromJSON([]byte(tt.body))
			if tt.wantKind == "" {
				if len(got) != 0 {
					t.Fatalf("candidates = %#v, want none", got)
				}
				return
			}
			if len(got) != 1 || got[0].Kind != tt.wantKind || got[0].Suffix != tt.wantSuffix || !got[0].Provisional {
				t.Fatalf("candidates = %#v, want provisional %s", got, tt.wantKind)
			}
		})
	}
}

func TestCandidateProcessJSONTags(t *testing.T) {
	data, err := json.Marshal(Candidate{SourceAppID: "wx456", Attribution: AttributionUnattributed, Provisional: true})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, want := range []string{`"source_app_id":"wx456"`, `"attribution":"unattributed"`, `"provisional":true`} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("marshaled candidate = %q, want %q", encoded, want)
		}
	}
}

func TestUnattributedGenerationInvalidatesOnlyOnClear(t *testing.T) {
	store := NewStore()
	generation := store.UnattributedGeneration()
	store.SetTarget(Target{AppID: "wx123"})
	candidate := Candidate{URL: "https://cdn.example.com/v.mp4", Kind: "video"}
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); !ok {
		t.Fatal("target selection discarded first-page response")
	}
	store.Clear("wx123")
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); ok {
		t.Fatal("old response repopulated cleared list")
	}
	generation = store.UnattributedGeneration()
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); !ok {
		t.Fatal("new response after clear was discarded")
	}
	store.ClearUnattributed()
	if _, ok := store.AddUnattributedForGeneration(generation, candidate); ok {
		t.Fatal("old response survived ClearUnattributed")
	}
}

func TestClassifyRecognizesFLV(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		rawURL      string
		wantKind    string
		wantSuffix  string
	}{
		{name: "flv content type", contentType: "video/x-flv", rawURL: "https://live.example.com/channel/9", wantKind: "video", wantSuffix: ".flv"},
		{name: "flv application type", contentType: "application/x-flv; charset=binary", rawURL: "https://live.example.com/channel/9", wantKind: "video", wantSuffix: ".flv"},
		{name: "flv url extension", rawURL: "https://play-t-live.example.com/7559_esd.flv?auth=1", wantKind: "video", wantSuffix: ".flv"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			kind, suffix := Classify(tt.contentType, tt.rawURL)
			if kind != tt.wantKind || suffix != tt.wantSuffix {
				t.Fatalf("Classify(%q, %q) = (%q, %q), want (%q, %q)", tt.contentType, tt.rawURL, kind, suffix, tt.wantKind, tt.wantSuffix)
			}
		})
	}
}

func TestLooksLikeLiveFLV(t *testing.T) {
	cases := []struct {
		name          string
		contentType   string
		contentLength int64
		rawURL        string
		want          bool
	}{
		{name: "flv type without length", contentType: "video/x-flv", rawURL: "https://live.example.com/a", want: true},
		{name: "flv url without length", rawURL: "https://live.example.com/a.flv", want: true},
		{name: "flv url with length", rawURL: "https://cdn.example.com/a.flv", contentLength: 1024, want: false},
		{name: "mp4 url", rawURL: "https://cdn.example.com/a.mp4", want: false},
		{name: "mp4 url with length", rawURL: "https://cdn.example.com/a.mp4", contentLength: 1024, want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := LooksLikeLiveFLV(tt.contentType, tt.contentLength, tt.rawURL); got != tt.want {
				t.Fatalf("LooksLikeLiveFLV(%q, %d, %q) = %v, want %v", tt.contentType, tt.contentLength, tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestExtractMediaURLsFromJSONMarksFLVLive(t *testing.T) {
	body := []byte(`{"data":{"videoUrl":"https://play-t-live.example.com/7559184446249188_esd.flv"}}`)
	got := ExtractMediaURLsFromJSON(body)
	if len(got) != 1 {
		t.Fatalf("candidates = %#v, want one flv candidate", got)
	}
	candidate := got[0]
	if candidate.Kind != "video" || candidate.Suffix != ".flv" || !candidate.Live {
		t.Fatalf("flv candidate = %#v, want video/.flv marked live", candidate)
	}
}

func TestCandidateLiveJSONTag(t *testing.T) {
	data, err := json.Marshal(Candidate{Live: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"live":true`) {
		t.Fatalf("marshaled candidate = %q, want live tag", data)
	}
}

func TestStoreSetLiveUpdatesTargetAndUnattributed(t *testing.T) {
	store := NewStore()
	store.SetTarget(Target{AppID: "wx123"})
	_, generation := store.Observe("wx123")
	mediaURL := "https://play-t-live.example.com/7559_esd.flv"

	// 未归属列表按 URL 去重，候选 ID 不带 AppID；目标列表的 ID 带 AppID。
	if _, ok := store.AddUnattributed(Candidate{URL: mediaURL, Kind: "video", Suffix: ".flv", Live: true}); !ok {
		t.Fatal("unattributed candidate should be admitted")
	}
	if _, ok := store.AddForTarget(generation, Candidate{AppID: "wx123", URL: mediaURL, Kind: "video", Suffix: ".flv", Live: true}, nil); !ok {
		t.Fatal("target candidate should be admitted")
	}
	targetID := CandidateID("wx123", mediaURL)
	unattributedID := CandidateID("", mediaURL)
	if targetID == unattributedID {
		t.Fatal("test requires distinct target/unattributed IDs")
	}

	if !store.SetLive(targetID, false) {
		t.Fatal("SetLive on a target candidate should report a hit")
	}
	if target, ok := store.Get(targetID); !ok || target.Live {
		t.Fatalf("target candidate Live = %#v, want finite video", target)
	}
	unattributed := store.Unattributed()
	if len(unattributed) != 1 || unattributed[0].Live {
		t.Fatalf("unattributed candidate Live = %#v, want propagated finite video", unattributed)
	}

	// 反向回写（有限文件确认为直播）同样按 URL 传播到另一处列表。
	if !store.SetLive(unattributedID, true) {
		t.Fatal("SetLive on an unattributed candidate should report a hit")
	}
	if target, _ := store.Get(targetID); !target.Live {
		t.Fatalf("target candidate Live = %v, want propagated live", target.Live)
	}
	if got := store.Unattributed(); len(got) != 1 || !got[0].Live {
		t.Fatalf("unattributed candidate = %#v, want live", got)
	}

	// 命中但值未变化时仍返回 true；未知 ID 返回 false。
	if !store.SetLive(targetID, true) {
		t.Fatal("SetLive should report a hit even when the value is unchanged")
	}
	if store.SetLive("missing-candidate", true) {
		t.Fatal("SetLive should report no hit for an unknown candidate")
	}
	if store.SetLive("   ", true) {
		t.Fatal("SetLive should ignore blank candidate IDs")
	}
}

func TestExtractMediaURLsFromJSONRichTitlesAndParentInheritance(t *testing.T) {
	body := []byte(`{
		"data": {
			"content": [
				{
					"goodsName": "  东方甄选原切牛排 \n 礼盒装  ",
					"videoInfo": {
						"label": "hd",
						"videoUrl": "https://v3.eastbuy.com/vod/d8fb84bccd044dd6885dfd56332a8292.mp4"
					}
				},
				{
					"roomName": "双11生鲜专场直播",
					"liveStreamUrl": "https://play-t-live.example.com/7559179538913600_d3fhd.flv"
				},
				{
					"title": "640",
					"imageUrl": "https://mmbiz.qpic.cn/mmbiz_jpg/abc123/640.jpg"
				}
			]
		}
	}`)
	candidates := ExtractMediaURLsFromJSON(body)
	if len(candidates) != 3 {
		t.Fatalf("got %d candidates, want 3: %#v", len(candidates), candidates)
	}
	byURL := map[string]Candidate{}
	for _, c := range candidates {
		byURL[c.URL] = c
	}
	if got := byURL["https://v3.eastbuy.com/vod/d8fb84bccd044dd6885dfd56332a8292.mp4"].Title; got != "东方甄选原切牛排 礼盒装" {
		t.Fatalf("nested video title = %q, want parent goodsName", got)
	}
	if got := byURL["https://play-t-live.example.com/7559179538913600_d3fhd.flv"].Title; got != "双11生鲜专场直播" {
		t.Fatalf("live stream title = %q, want roomName", got)
	}
	if got := byURL["https://mmbiz.qpic.cn/mmbiz_jpg/abc123/640.jpg"].Title; got != "" {
		t.Fatalf("numeric opaque title should be ignored, got %q", got)
	}
}

func TestExtractDispositionAndHTMLTitles(t *testing.T) {
	if got := ExtractDispositionTitle(`attachment; filename*=UTF-8''%E6%98%A5%E5%AD%A3%E8%AF%BE%E7%A8%8B%E8%AE%B2%E8%A7%A3.mp4`); got != "春季课程讲解" {
		t.Fatalf("ExtractDispositionTitle RFC5987 = %q", got)
	}
	if got := ExtractDispositionTitle(`inline; filename="af8f312f05994fd6bcd87c419b312b62.mp4"`); got != "" {
		t.Fatalf("ExtractDispositionTitle opaque hash should be ignored, got %q", got)
	}
	htmlBody := []byte(`<html><head><title>旧标题</title><meta property="og:title" content="高数精讲第1课 &amp; 笔记"></head><body><video src="https://cdn.example.com/a.mp4"></video></body></html>`)
	if got := ExtractHTMLTitle(htmlBody); got != "高数精讲第1课 & 笔记" {
		t.Fatalf("ExtractHTMLTitle = %q", got)
	}
}

func TestExtractURLTitle(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://cdn.eastbuy.com/media/%E5%AE%A3%E4%BC%A0%E7%89%87.mp4", "宣传片"},
		{"https://cdn.eastbuy.com/assets/spring-banner.jpg?v=2", "spring-banner"},
		{"https://cdn.eastbuy.com/vod/d8fb84bccd044dd6885dfd56332a8292.mp4", ""},
		{"https://play-t-live.example.com/7559184446249188_esd.flv", ""},
		{"https://cdn.example.com/live/index.m3u8", ""},
		{"https://cdn.example.com/stream.flv", ""},
		{"https://cdn.example.com/video.mp4", ""},
		{"https://cdn.example.com/a.jpg", ""},
		{"https://cdn.example.com/宣传.mp4", "宣传"},
	}
	for _, tc := range cases {
		if got := ExtractURLTitle(tc.url); got != tc.want {
			t.Errorf("ExtractURLTitle(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestStoreSyncsTitleAcrossTargetAndUnattributed(t *testing.T) {
	// 1. 目标 JSON 先带标题进入 -> 后续缺少 Referer 的未归属响应自动继承该标题。
	store := NewStore()
	store.SetTarget(Target{AppID: "wx123"})
	_, gen := store.Observe("wx123")
	u1 := "https://v3.eastbuy.com/vod/d8fb84bccd044dd6885dfd56332a8292.mp4"
	if _, ok := store.AddForTarget(gen, Candidate{AppID: "wx123", URL: u1, Title: "原切牛排讲解", Kind: "video", Suffix: ".mp4"}, nil); !ok {
		t.Fatal("failed to add target candidate")
	}
	gotU, ok := store.AddUnattributed(Candidate{URL: u1, Source: "response", Kind: "video", Suffix: ".mp4"})
	if !ok || gotU.Title != "原切牛排讲解" {
		t.Fatalf("unattributed candidate did not inherit target title: %#v", gotU)
	}

	// 2. 未归属先带标题进入（或后补）-> 目标候选自动同步标题。
	u2 := "https://cdn.eastbuy.com/media/998877665544.mp4"
	if _, ok := store.AddForTarget(gen, Candidate{AppID: "wx123", URL: u2, Kind: "video", Suffix: ".mp4"}, nil); !ok {
		t.Fatal("failed to add untitled target candidate")
	}
	if _, ok := store.AddUnattributed(Candidate{URL: u2, Title: "新品预热视频", Kind: "video", Suffix: ".mp4"}); !ok {
		t.Fatal("failed to add titled unattributed candidate")
	}
	gotT, ok := store.Get(CandidateID("wx123", u2))
	if !ok || gotT.Title != "新品预热视频" {
		t.Fatalf("target candidate did not receive propagated title: %#v", gotT)
	}
}
