package miniprogram

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"html"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Target struct {
	AppID      string `json:"app_id"`
	OriginalID string `json:"original_id"`
	LinkPrefix string `json:"link_prefix"`
}

const (
	// AttributionConfirmed 表示候选来自等于目标 AppID 的 page-frame Referer。
	AttributionConfirmed = "confirmed"
	// AttributionUnattributed 表示候选无法归入当前目标 AppID，只进入未归属列表。
	AttributionUnattributed = "unattributed"
)

type Candidate struct {
	ID          string `json:"id"`
	AppID       string `json:"app_id"`
	SourceAppID string `json:"source_app_id,omitempty"`
	Attribution string `json:"attribution,omitempty"`
	Provisional bool   `json:"provisional,omitempty"`
	// Live 表示疑似 HTTP-FLV 直播流：.flv 直链，或 FLV Content-Type 且没有可用的 Content-Length。
	// 它没有总时长，下载等于持续录制、不会自动结束；下载时会再用响应头和 FLV 魔数确认。
	Live          bool              `json:"live,omitempty"`
	Title         string            `json:"title,omitempty"`
	URL           string            `json:"url"`
	SourceURL     string            `json:"source_url"`
	Source        string            `json:"source"`
	FieldPath     string            `json:"field_path,omitempty"`
	ContentType   string            `json:"content_type"`
	ContentLength int64             `json:"content_length"`
	Kind          string            `json:"kind"`
	Suffix        string            `json:"suffix"`
	Headers       map[string]string `json:"headers,omitempty"`
	CachedPath    string            `json:"cached_path,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// unattributedLimit 限制未归属列表的容量，避免长时间嗅探后无限增长。
const unattributedLimit = 500

type Store struct {
	mu                     sync.RWMutex
	items                  map[string]Candidate
	order                  []string
	unattributed           map[string]Candidate
	unattributedOrder      []string
	unattributedGeneration uint64
	target                 Target
	generation             uint64
	programs               []Program
	programHosts           map[string]map[string]int
	OnCandidateAdded       func(Candidate)
}

func NewStore() *Store {
	return &Store{
		items:             make(map[string]Candidate),
		unattributed:      make(map[string]Candidate),
		unattributedOrder: nil,
		programHosts:      make(map[string]map[string]int),
	}
}

func (s *Store) Add(candidate Candidate) (Candidate, bool) {
	return s.add(candidate, nil, nil)
}

func (s *Store) AddForTarget(generation uint64, candidate Candidate, cache func() string) (Candidate, bool) {
	return s.add(candidate, &generation, cache)
}

// AddUnattributed 把无法归入当前目标 AppID 的媒体放入未归属列表。
// 该列表与目标和代次无关：切换目标不会清空它；重复 URL 会合并最新请求头，
// 便于用最近一次真实响应的 Referer/Cookie 下载防盗链资源；超过上限时丢弃最旧条目。
func (s *Store) AddUnattributed(candidate Candidate) (Candidate, bool) {
	return s.addUnattributed(candidate, nil)
}

// UnattributedGeneration 只随清空变化，选择或切换目标不使首屏媒体失效。
func (s *Store) UnattributedGeneration() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unattributedGeneration
}

func (s *Store) AddUnattributedForGeneration(generation uint64, candidate Candidate) (Candidate, bool) {
	return s.addUnattributed(candidate, &generation)
}

func (s *Store) addUnattributed(candidate Candidate, generation *uint64) (Candidate, bool) {
	if candidate.URL == "" {
		return candidate, false
	}
	if candidate.ID == "" {
		// 未归属列表按 URL 归一去重：同一媒体由不同 H5 来源重复请求时只保留一条。
		candidate.ID = CandidateID("", candidate.URL)
	}
	if candidate.Attribution == "" {
		candidate.Attribution = AttributionUnattributed
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	s.mu.Lock()
	if generation != nil && *generation != s.unattributedGeneration {
		s.mu.Unlock()
		return candidate, false
	}
	if existing, exists := s.unattributed[candidate.ID]; exists {
		changed := false
		if candidate.Title == "" {
			if targetTitle := s.findTargetTitleLocked(candidate.URL); targetTitle != "" {
				candidate.Title = targetTitle
			}
		}
		// 页面推测不能覆盖真实媒体请求中观察到的防盗链凭据。
		preferExistingResponse := existing.Source == "response" && candidate.Source == "html"
		if !preferExistingResponse && candidate.Source != "" && candidate.Source != existing.Source {
			existing.Source = candidate.Source
			changed = true
		}
		if !preferExistingResponse && len(candidate.Headers) > 0 {
			existing.Headers = candidate.Headers
			changed = true
		}
		if !preferExistingResponse && candidate.SourceURL != "" && candidate.SourceURL != existing.SourceURL {
			existing.SourceURL = candidate.SourceURL
			changed = true
		}
		if candidate.ContentType != "" && candidate.ContentType != existing.ContentType {
			existing.ContentType = candidate.ContentType
			changed = true
		}
		if candidate.ContentLength > 0 && candidate.ContentLength != existing.ContentLength {
			existing.ContentLength = candidate.ContentLength
			changed = true
		}
		if candidate.SourceAppID != "" && candidate.SourceAppID != existing.SourceAppID {
			existing.SourceAppID = candidate.SourceAppID
			changed = true
		}
		if candidate.Kind != "" && candidate.Kind != existing.Kind {
			existing.Kind = candidate.Kind
			changed = true
		}
		if candidate.Suffix != "" && existing.Suffix == "" {
			existing.Suffix = candidate.Suffix
			changed = true
		}
		// 真实响应比页面/JSON 推测更可信：用响应判定覆盖直播标记与后缀。
		if candidate.Source == "response" {
			if candidate.Live != existing.Live {
				existing.Live = candidate.Live
				changed = true
			}
			if candidate.Suffix != "" && candidate.Suffix != existing.Suffix {
				existing.Suffix = candidate.Suffix
				changed = true
			}
		}
		if candidate.Title != "" && existing.Title == "" {
			existing.Title = candidate.Title
			changed = true
		}
		if candidate.FieldPath != "" && existing.FieldPath == "" {
			existing.FieldPath = candidate.FieldPath
			changed = true
		}
		if changed {
			s.unattributed[candidate.ID] = existing
		}
		if existing.Title != "" {
			s.propagateTitleToTargetLocked(candidate.URL, existing.Title)
		}
		s.mu.Unlock()
		return existing, changed
	}
	if candidate.Title == "" {
		if targetTitle := s.findTargetTitleLocked(candidate.URL); targetTitle != "" {
			candidate.Title = targetTitle
		}
	}
	s.unattributed[candidate.ID] = candidate
	s.unattributedOrder = append(s.unattributedOrder, candidate.ID)
	for len(s.unattributedOrder) > unattributedLimit {
		delete(s.unattributed, s.unattributedOrder[0])
		s.unattributedOrder = s.unattributedOrder[1:]
	}
	if candidate.Title != "" {
		s.propagateTitleToTargetLocked(candidate.URL, candidate.Title)
	}
	s.mu.Unlock()
	return candidate, true
}

// Unattributed 返回未归属列表的副本，按首次出现顺序排列。
func (s *Store) Unattributed() []Candidate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.unattributedOrder) == 0 {
		return nil
	}
	out := make([]Candidate, 0, len(s.unattributedOrder))
	for _, id := range s.unattributedOrder {
		if item, ok := s.unattributed[id]; ok {
			out = append(out, item)
		}
	}
	return out
}

func (s *Store) ClearUnattributed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unattributedGeneration++
	s.unattributed = make(map[string]Candidate)
	s.unattributedOrder = nil
}

func (s *Store) add(candidate Candidate, generation *uint64, cache func() string) (Candidate, bool) {
	if candidate.URL == "" {
		return candidate, false
	}
	if candidate.ID == "" {
		candidate.ID = CandidateID(candidate.AppID, candidate.URL)
	}
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now()
	}
	s.mu.Lock()
	if generation != nil && (*generation != s.generation || s.target.AppID == "" || candidate.AppID != s.target.AppID) {
		s.mu.Unlock()
		return candidate, false
	}
	if cache != nil {
		candidate.CachedPath = cache()
	}
	unattrID := CandidateID("", candidate.URL)
	if candidate.Title == "" {
		if u, ok := s.unattributed[unattrID]; ok && u.Title != "" {
			candidate.Title = u.Title
		}
	}
	if existing, exists := s.items[candidate.ID]; exists {
		changed := false
		if candidate.Source == "response" {
			existing.Source = candidate.Source
			existing.Headers = candidate.Headers
			existing.ContentType = candidate.ContentType
			existing.ContentLength = candidate.ContentLength
			existing.Live = candidate.Live
			changed = true
		}
		if candidate.CachedPath != "" {
			existing.CachedPath = candidate.CachedPath
			changed = true
		}
		if candidate.ContentLength > 0 && existing.ContentLength == 0 {
			existing.ContentLength = candidate.ContentLength
			changed = true
		}
		if candidate.ContentType != "" && existing.ContentType == "" {
			existing.ContentType = candidate.ContentType
			changed = true
		}
		if candidate.Title != "" && existing.Title == "" {
			existing.Title = candidate.Title
			changed = true
		}
		if len(candidate.Headers) > 0 && len(existing.Headers) == 0 && existing.Source != "response" {
			existing.Headers = candidate.Headers
			changed = true
		}
		if changed {
			s.items[candidate.ID] = existing
		}
		if existing.Title != "" {
			s.propagateTitleToUnattributedLocked(unattrID, existing.Title)
		}
		s.mu.Unlock()
		return existing, changed
	}
	s.items[candidate.ID] = candidate
	s.order = append(s.order, candidate.ID)
	if candidate.Title != "" {
		s.propagateTitleToUnattributedLocked(unattrID, candidate.Title)
	}
	s.mu.Unlock()
	if s.OnCandidateAdded != nil {
		s.OnCandidateAdded(candidate)
	}
	return candidate, true
}

func (s *Store) List(appID string) []Candidate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Candidate
	for _, id := range s.order {
		item, ok := s.items[id]
		if !ok {
			continue
		}
		if appID != "" && item.AppID != appID {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (s *Store) Get(id string) (Candidate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

func (s *Store) SetCachedPath(id, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return
	}
	item.CachedPath = path
	s.items[id] = item
}

// SetLive 用下载阶段确认的直播状态回写候选，修正“疑似直播”与实际响应不一致的条目。
// 目标候选与来源未确认候选都会更新。两个列表的候选 ID 计算方式不同（是否带 AppID），
// 同一媒体在两处的 ID 可能不同，因此命中任一列表后按 URL 传播到另一处。
// 返回是否命中了候选。
func (s *Store) SetLive(id string, live bool) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	urls := make(map[string]struct{}, 2)
	if item, ok := s.items[id]; ok {
		urls[item.URL] = struct{}{}
	}
	if item, ok := s.unattributed[id]; ok {
		urls[item.URL] = struct{}{}
	}
	if len(urls) == 0 {
		return false
	}
	matches := func(item Candidate) bool {
		if item.ID == id {
			return true
		}
		_, ok := urls[item.URL]
		return ok
	}
	for key, item := range s.items {
		if item.Live == live || !matches(item) {
			continue
		}
		item.Live = live
		s.items[key] = item
	}
	for key, item := range s.unattributed {
		if item.Live == live || !matches(item) {
			continue
		}
		item.Live = live
		s.unattributed[key] = item
	}
	return true
}

// Clear 按“清空列表”语义工作：清空目标候选的同时清空未归属列表，并让在途响应失效。
// 切换目标请用 SetTarget：它只清空目标候选，保留与目标无关的未归属列表。
func (s *Store) Clear(appID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clearAll := appID == ""
	clearUnattributed := clearAll || appID == s.target.AppID
	if clearUnattributed {
		s.generation++
		s.unattributedGeneration++
		s.unattributed = make(map[string]Candidate)
		s.unattributedOrder = nil
	}
	if clearAll {
		s.items = make(map[string]Candidate)
		s.order = nil
		return
	}
	filtered := s.order[:0]
	for _, id := range s.order {
		item, ok := s.items[id]
		if !ok {
			continue
		}
		if item.AppID == appID {
			delete(s.items, id)
			continue
		}
		filtered = append(filtered, id)
	}
	s.order = filtered
}

func CandidateID(appID, rawURL string) string {
	h := sha1.Sum([]byte(appID + "\n" + rawURL))
	return hex.EncodeToString(h[:])
}

func ClassifyByContentType(contentType string) (kind, suffix string) {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	switch mediaType {
	case "image/jpeg":
		return "image", ".jpg"
	case "image/png":
		return "image", ".png"
	case "image/webp":
		return "image", ".webp"
	case "image/gif":
		return "image", ".gif"
	case "image/bmp":
		return "image", ".bmp"
	case "image/avif":
		return "image", ".avif"
	case "video/mp4":
		return "video", ".mp4"
	case "video/webm":
		return "video", ".webm"
	case "video/x-flv", "video/flv", "application/x-flv":
		return "video", ".flv"
	case "video/quicktime":
		return "video", ".mov"
	case "audio/mpeg", "audio/mp3":
		return "audio", ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return "audio", ".m4a"
	case "audio/aac":
		return "audio", ".aac"
	case "audio/wav", "audio/x-wav", "audio/vnd.wave":
		return "audio", ".wav"
	case "audio/ogg":
		return "audio", ".ogg"
	case "audio/flac", "audio/x-flac":
		return "audio", ".flac"
	case "audio/opus":
		return "audio", ".opus"
	case "audio/amr":
		return "audio", ".amr"
	case "application/vnd.apple.mpegurl", "application/x-mpegurl", "audio/x-mpegurl", "application/x-mpeg":
		return "m3u8", ".m3u8"
	case "video/mp2t":
		return "segment", ".ts"
	default:
		return "", ""
	}
}

func ClassifyByURL(rawURL string) (kind, suffix string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	ext := strings.ToLower(path.Ext(u.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".avif":
		if ext == ".jpeg" {
			return "image", ".jpg"
		}
		return "image", ext
	case ".mp4", ".webm", ".mov", ".flv":
		return "video", ext
	case ".mp3", ".m4a", ".aac", ".wav", ".ogg", ".flac", ".opus", ".amr":
		return "audio", ext
	case ".m3u8":
		return "m3u8", ".m3u8"
	case ".ts":
		return "segment", ".ts"
	default:
		return "", ""
	}
}

func Classify(contentType, rawURL string) (kind, suffix string) {
	if kind, suffix = ClassifyByContentType(contentType); kind != "" {
		return kind, suffix
	}
	kind, suffix = ClassifyByURL(rawURL)
	if kind == "audio" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "video/") {
		return "", ""
	}
	return kind, suffix
}

// IsFLVContentType 判断响应 Content-Type 是否属于 HTTP-FLV。
func IsFLVContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	switch mediaType {
	case "video/x-flv", "video/flv", "application/x-flv":
		return true
	}
	return false
}

// IsFLVURL 判断 URL 路径扩展名是否为 .flv。
func IsFLVURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(path.Ext(u.Path), ".flv")
}

// LooksLikeLiveFLV 判断媒体是否疑似 HTTP-FLV 直播流。
// 带 Content-Length 的响应是可以完整下载的有限文件，不算直播；长度未知时，
// FLV Content-Type 或 .flv 地址都按疑似直播处理——微信小程序常用 HTTP-FLV 做低延迟直播。
func LooksLikeLiveFLV(contentType string, contentLength int64, rawURL string) bool {
	if contentLength > 0 {
		return false
	}
	if IsFLVContentType(contentType) {
		return true
	}
	return IsFLVURL(rawURL)
}

func CleanHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	forbidden := map[string]struct{}{
		"accept-encoding":     {},
		"connection":          {},
		"content-length":      {},
		"host":                {},
		"if-match":            {},
		"if-modified-since":   {},
		"if-none-match":       {},
		"if-range":            {},
		"if-unmodified-since": {},
		"keep-alive":          {},
		"proxy-connection":    {},
		"proxy-authorization": {},
		"proxy-authenticate":  {},
		"range":               {},
		"transfer-encoding":   {},
	}
	clean := make(map[string]string)
	for key, values := range header {
		if _, ok := forbidden[strings.ToLower(key)]; ok || len(values) == 0 {
			continue
		}
		value := strings.TrimSpace(values[0])
		if value == "" {
			continue
		}
		clean[http.CanonicalHeaderKey(key)] = value
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// HeadersForURL 只在同源 URL 之间复用凭据，跨源只保留公开的请求元数据。
func HeadersForURL(headers map[string]string, sourceURL, targetURL string) map[string]string {
	source, err1 := url.Parse(sourceURL)
	target, err2 := url.Parse(targetURL)
	sameOrigin := err1 == nil && err2 == nil && source.Host != "" && target.Host != "" && strings.EqualFold(source.Scheme, target.Scheme) && strings.EqualFold(source.Host, target.Host)
	out := make(map[string]string)
	for key, value := range headers {
		switch strings.ToLower(key) {
		case "user-agent", "referer", "accept", "accept-language":
			out[key] = value
		default:
			if sameOrigin {
				out[key] = value
			}
		}
	}
	return out
}

var contentRangeRegexp = regexp.MustCompile(`^bytes ([0-9]+)-([0-9]+)/([0-9]+)$`)

func ParseContentRange(value string) (start, end, total int64, ok bool) {
	match := contentRangeRegexp.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return
	}
	var err error
	if start, err = strconv.ParseInt(match[1], 10, 64); err != nil {
		return
	}
	if end, err = strconv.ParseInt(match[2], 10, 64); err != nil {
		return
	}
	if total, err = strconv.ParseInt(match[3], 10, 64); err != nil {
		return
	}
	ok = start >= 0 && end >= start && total > end
	return
}

var mediaURLRegexp = regexp.MustCompile(`https?://[^\s"'<>\\]+`)

// MediaURLsInText 从 H5 页面/脚本文本中提取绝对媒体 URL。
// 只保留视频、m3u8 和音频，避免图片图标造成噪音；结果按 URL 排序便于测试与展示。
func MediaURLsInText(text string) []Candidate {
	if text == "" {
		return nil
	}
	seen := make(map[string]Candidate)
	for _, match := range mediaURLRegexp.FindAllString(text, -1) {
		raw := strings.TrimRight(match, ".,);]}\"'")
		if raw == "" {
			continue
		}
		kind, suffix := Classify("", raw)
		if kind != "video" && kind != "m3u8" && kind != "audio" {
			continue
		}
		if _, exists := seen[raw]; exists {
			continue
		}
		seen[raw] = Candidate{URL: raw, Source: "html", Kind: kind, Suffix: suffix, Live: IsFLVURL(raw)}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]Candidate, 0, len(seen))
	for _, candidate := range seen {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

func ExtractMediaURLsFromJSON(body []byte) []Candidate {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil
	}
	seen := make(map[string]Candidate)
	walkJSON("", v, "", seen)
	out := make([]Candidate, 0, len(seen))
	for _, candidate := range seen {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URL == out[j].URL {
			return out[i].FieldPath < out[j].FieldPath
		}
		return out[i].URL < out[j].URL
	})
	return out
}

type jsonTokenField struct {
	Key   string
	Value string
}

func walkJSON(fieldPath string, v any, nearbyTitle string, seen map[string]Candidate) {
	walkJSONRanked(fieldPath, v, nearbyTitle, 100, seen)
}

func walkJSONRanked(fieldPath string, v any, nearbyTitle string, nearbyRank int, seen map[string]Candidate) {
	switch value := v.(type) {
	case map[string]any:
		if title, rank := jsonObjectTitleRanked(value); title != "" && (nearbyTitle == "" || rank <= nearbyRank || rank <= 2) {
			nearbyTitle = title
			nearbyRank = rank
		}
		tokens := jsonTokenFields(value)
		for k, child := range value {
			nextPath := k
			if fieldPath != "" {
				nextPath = fieldPath + "." + k
			}
			if text, ok := child.(string); ok {
				walkJSONString(nextPath, text, nearbyTitle, tokens, seen)
				continue
			}
			walkJSONRanked(nextPath, child, nearbyTitle, nearbyRank, seen)
		}
	case []any:
		nextPath := fieldPath + "[]"
		if fieldPath == "" {
			nextPath = "[]"
		}
		for _, child := range value {
			walkJSONRanked(nextPath, child, nearbyTitle, nearbyRank, seen)
		}
	case string:
		walkJSONString(fieldPath, value, nearbyTitle, nil, seen)
	}
}

// jsonTokenFields 收集同一对象内形如 "urlToken"/"token"/"playToken"/"authKey"
// 且以 ? 或 & 开头的相邻 token 字段，用于拼接到 URL 字段上。
func jsonTokenFields(value map[string]any) []jsonTokenField {
	var tokens []jsonTokenField
	for key, raw := range value {
		text, ok := raw.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" || (text[0] != '?' && text[0] != '&') {
			continue
		}
		if !looksLikeJSONTokenField(key) {
			continue
		}
		tokens = append(tokens, jsonTokenField{Key: key, Value: text})
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Key < tokens[j].Key })
	return tokens
}

func looksLikeJSONTokenField(key string) bool {
	compact := compactJSONKey(key)
	for _, token := range []string{"token", "authkey", "signature", "sign", "auth"} {
		if strings.Contains(compact, token) {
			return true
		}
	}
	return false
}

func looksLikeJSONURLField(key string) bool {
	compact := compactJSONKey(key)
	if compact == "src" || strings.HasSuffix(compact, "src") {
		return true
	}
	for _, token := range []string{"url", "uri", "link"} {
		if strings.Contains(compact, token) {
			return true
		}
	}
	return false
}

func compactJSONKey(key string) string {
	key = strings.ToLower(key)
	return strings.NewReplacer("_", "", "-", "").Replace(key)
}

// matchJSONToken 为 URL 字段挑选同一对象内可拼接的 token。
// 优先按字段名配对（playUrl + playUrlToken），其次把唯一的 token 拼给明显的 URL 字段。
func matchJSONToken(fieldKey string, tokens []jsonTokenField) (string, bool) {
	if len(tokens) == 0 || !looksLikeJSONURLField(fieldKey) {
		return "", false
	}
	compact := compactJSONKey(fieldKey)
	for _, token := range tokens {
		compactToken := compactJSONKey(token.Key)
		if compactToken == compact+"token" || strings.HasPrefix(compactToken, compact) {
			return token.Value, true
		}
	}
	if len(tokens) == 1 && (compact == "url" || compact == "src" || strings.HasSuffix(compact, "url") || strings.HasSuffix(compact, "src")) {
		return tokens[0].Value, true
	}
	return "", false
}

// appendJSONToken 把 ?xxx / &xxx 风格的 token 拼到 URL 上，保证只有一个查询分隔符。
func appendJSONToken(rawURL, token string) string {
	switch {
	case strings.HasPrefix(token, "?"):
		if strings.Contains(rawURL, "?") {
			return rawURL + "&" + strings.TrimPrefix(token, "?")
		}
		return rawURL + token
	case strings.HasPrefix(token, "&"):
		if strings.Contains(rawURL, "?") {
			return rawURL + token
		}
		return rawURL + "?" + strings.TrimPrefix(token, "&")
	default:
		return rawURL
	}
}

func walkJSONString(fieldPath, value, nearbyTitle string, tokens []jsonTokenField, seen map[string]Candidate) {
	key := strings.ToLower(fieldPath)
	for _, match := range mediaURLRegexp.FindAllString(value, -1) {
		rawURL := strings.TrimRight(match, ".,);]")
		urls := []string{rawURL}
		if token, ok := matchJSONToken(jsonFieldKey(fieldPath), tokens); ok {
			if combined := appendJSONToken(rawURL, token); combined != rawURL {
				urls = []string{combined}
			}
		}
		for _, urlValue := range urls {
			kind, suffix := Classify("", urlValue)
			provisional := false
			if kind == "" {
				// 无扩展名时只用强相关字段名做低置信推断，避免把普通链接当媒体。
				kind, suffix = provisionalMediaKind(key, urlValue)
				if kind == "" {
					continue
				}
				provisional = true
			}
			// JSON 提取同样排除分片，分片由 m3u8 下载流程处理。
			if kind == "segment" {
				continue
			}
			seen[urlValue+"|"+fieldPath] = Candidate{
				URL:         urlValue,
				Title:       nearbyTitle,
				Source:      "json",
				FieldPath:   fieldPath,
				Kind:        kind,
				Suffix:      suffix,
				Live:        IsFLVURL(urlValue),
				Provisional: provisional,
			}
		}
	}
}

// jsonFieldKey 取字段路径最后一段，用于相邻 token 字段配对。
func jsonFieldKey(fieldPath string) string {
	if idx := strings.LastIndex(fieldPath, "."); idx >= 0 {
		return fieldPath[idx+1:]
	}
	return fieldPath
}

// provisionalMediaKind 在地址无扩展名但字段名强相关时给出低置信类型。
func provisionalMediaKind(fieldPath, rawURL string) (string, string) {
	if !looksLikeStrongMediaField(fieldPath) || looksLikeImageField(fieldPath) {
		return "", ""
	}
	if strings.Contains(fieldPath, "m3u8") || strings.Contains(fieldPath, "hls") || strings.Contains(fieldPath, "stream") {
		return "m3u8", ".m3u8"
	}
	if u, err := url.Parse(rawURL); err == nil {
		pathLower := strings.ToLower(u.Path)
		if strings.Contains(pathLower, "m3u8") || strings.Contains(pathLower, "playlist") || strings.Contains(pathLower, "manifest") {
			return "m3u8", ".m3u8"
		}
	}
	return "video", ".mp4"
}

func looksLikeStrongMediaField(key string) bool {
	if key == "" {
		return false
	}
	for _, token := range []string{"video", "play", "stream", "media", "m3u8"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func looksLikeImageField(key string) bool {
	for _, token := range []string{
		"cover", "poster", "avatar", "thumb", "banner", "logo",
		"image", "img", "pic", "photo",
	} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func jsonObjectTitle(value map[string]any) string {
	title, _ := jsonObjectTitleRanked(value)
	return title
}

func jsonObjectTitleRanked(value map[string]any) (string, int) {
	bestRank := 100
	best := ""
	for key, raw := range value {
		text, ok := raw.(string)
		if !ok {
			continue
		}
		compact := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
		rank := 100
		switch compact {
		case "title", "videotitle", "audiotitle", "mediatitle", "livetitle", "roomtitle",
			"articletitle", "coursetitle", "lessontitle", "chaptertitle", "programtitle",
			"feedtitle", "posttitle", "headline", "subject", "caption":
			rank = 1
		case "goodsname", "productname", "spuname", "skuname", "itemname",
			"roomname", "livename", "coursename", "lessonname", "chaptername",
			"filename", "originalname", "displayname", "showname",
			"medianame", "videoname", "audioname", "albumname", "songname", "trackname",
			"topic", "name":
			rank = 2
		case "anchorname", "authorname", "nickname", "username", "sellername",
			"shopname", "storename", "desc", "description", "summary", "brief",
			"content", "text", "label":
			rank = 3
		}
		if rank >= 100 {
			continue
		}
		cleaned := CleanCandidateTitle(text)
		if cleaned == "" {
			continue
		}
		if rank == 3 && (strings.ContainsAny(text, "<>{}") || len([]rune(strings.TrimSpace(text))) > 120) {
			continue
		}
		if rank < bestRank || (rank == bestRank && cleaned < best) {
			bestRank = rank
			best = cleaned
		}
	}
	return best, bestRank
}

var (
	uuidRegexp       = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hexHashRegexp    = regexp.MustCompile(`(?i)^[0-9a-f]{12,}$`)
	digitsOnlyRegexp = regexp.MustCompile(`^[0-9]+$`)
	ogTitleRegexp    = regexp.MustCompile(`(?is)<meta[^>]+(?:property|name)\s*=\s*["']og:title["'][^>]+content\s*=\s*["']([^"']+)["']`)
	ogTitleRevRegexp = regexp.MustCompile(`(?is)<meta[^>]+content\s*=\s*["']([^"']+)["'][^>]+(?:property|name)\s*=\s*["']og:title["']`)
	htmlTitleRegexp  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// CleanCandidateTitle 清洗从 JSON/响应头/网页中提取的候选标题，过滤无意义哈希、URL 与过长正文。
func CleanCandidateTitle(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	cleaned := strings.Join(fields, " ")
	if mediaURLRegexp.MatchString(cleaned) {
		return ""
	}
	if strings.HasPrefix(cleaned, "{") || strings.HasPrefix(cleaned, "[") {
		return ""
	}
	if LooksLikeOpaqueID(cleaned) {
		return ""
	}
	runes := []rune(cleaned)
	if len(runes) > 60 {
		cleaned = strings.TrimSpace(string(runes[:60]))
	}
	return cleaned
}

// LooksLikeOpaqueID 判断字符串是否更像数字编号、UUID 或随机哈希，而非人类可读名称。
func LooksLikeOpaqueID(value string) bool {
	s := strings.TrimSpace(value)
	if s == "" {
		return true
	}
	if digitsOnlyRegexp.MatchString(s) || uuidRegexp.MatchString(s) || hexHashRegexp.MatchString(s) {
		return true
	}
	if strings.ContainsAny(s, " \t") {
		return false
	}
	hasCJK := false
	hasUpper := false
	hasLower := false
	hasDigit := false
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			hasCJK = true
			break
		}
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		} else if r >= 'a' && r <= 'z' {
			hasLower = true
		} else if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	if hasCJK {
		return false
	}
	// 无空格、无中文且长度 >= 20 的字母数字混合随机 token（含 UUID 片段或 base64url）视作不透明 ID。
	return len(s) >= 20 && hasDigit && (hasUpper || hasLower)
}

// ExtractDispositionTitle 从 HTTP Content-Disposition 头提取可读文件名（去掉扩展名并清洗）。
func ExtractDispositionTitle(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(params["filename"])
	if name == "" {
		return ""
	}
	if unescaped, err := url.PathUnescape(name); err == nil && strings.TrimSpace(unescaped) != "" {
		name = unescaped
	}
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	stem := strings.TrimSuffix(name, path.Ext(name))
	return CleanCandidateTitle(stem)
}

// ExtractHTMLTitle 从网页源码提取 og:title 或 <title> 作为资源标题提示。
func ExtractHTMLTitle(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	text := string(body)
	for _, re := range []*regexp.Regexp{ogTitleRegexp, ogTitleRevRegexp, htmlTitleRegexp} {
		if match := re.FindStringSubmatch(text); len(match) == 2 {
			if cleaned := CleanCandidateTitle(html.UnescapeString(match[1])); cleaned != "" {
				return cleaned
			}
		}
	}
	return ""
}

// ExtractURLTitle 从 URL 路径末段提取有意义的文件名（去掉扩展名与清晰度后缀），
// 过滤通用占位名、过短纯 ASCII 片段与不透明哈希 ID。
func ExtractURLTitle(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	trimmedPath := strings.TrimRight(u.Path, "/")
	if trimmedPath == "" || trimmedPath == "/" {
		return ""
	}
	base := path.Base(trimmedPath)
	if base == "" || base == "." || base == "/" {
		return ""
	}
	if unescaped, err := url.PathUnescape(base); err == nil && strings.TrimSpace(unescaped) != "" {
		base = unescaped
	}
	stem := strings.TrimSpace(strings.TrimSuffix(base, path.Ext(base)))
	if stem == "" {
		return ""
	}
	stem = stripURLQualitySuffix(stem)
	if isGenericURLStem(stem) {
		return ""
	}
	if isASCIIOnly(stem) && len(stem) <= 2 {
		return ""
	}
	return CleanCandidateTitle(stem)
}

func stripURLQualitySuffix(stem string) string {
	lower := strings.ToLower(stem)
	for _, sfx := range []string{
		"_d3fhd", "_efhd", "_fhd", "_1080p",
		"_ehd", "_hd", "_720p",
		"_esd", "_sd", "_480p",
		"_eld", "_ld", "_360p",
		"_uhd", "_4k", "_2160p",
		"_orig", "_origin",
	} {
		if strings.HasSuffix(lower, sfx) && len(stem) > len(sfx) {
			return stem[:len(stem)-len(sfx)]
		}
	}
	return stem
}

func isGenericURLStem(stem string) bool {
	switch strings.ToLower(strings.TrimSpace(stem)) {
	case "index", "playlist", "master", "manifest", "chunklist",
		"stream", "live", "livestream", "live-stream", "live_stream",
		"video", "audio", "image", "img", "pic", "photo", "file", "media",
		"download", "play", "default", "hls", "m3u8", "vod", "data",
		"raw", "bin", "blob", "clip", "part", "seg", "segment", "resume",
		"cover", "asset", "item", "resource", "content":
		return true
	}
	return false
}

func isASCIIOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (s *Store) findTargetTitleLocked(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if s.target.AppID != "" {
		if item, ok := s.items[CandidateID(s.target.AppID, rawURL)]; ok && item.Title != "" {
			return item.Title
		}
	}
	for _, item := range s.items {
		if item.URL == rawURL && item.Title != "" {
			return item.Title
		}
	}
	return ""
}

func (s *Store) propagateTitleToTargetLocked(rawURL, title string) {
	if rawURL == "" || title == "" {
		return
	}
	for id, item := range s.items {
		if item.URL == rawURL && item.Title == "" {
			item.Title = title
			s.items[id] = item
		}
	}
}

func (s *Store) propagateTitleToUnattributedLocked(unattrID, title string) {
	if unattrID == "" || title == "" {
		return
	}
	if item, ok := s.unattributed[unattrID]; ok && item.Title == "" {
		item.Title = title
		s.unattributed[unattrID] = item
	}
}
