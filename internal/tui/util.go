package tui

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/MuZiJin701/wx-mini-video/internal/miniprogram"
)

var categories = []category{categoryAll, categoryImage, categoryVideo, categoryAudio, categoryLive, categorySegments}

func categoryDisplay(value category) string {
	switch value {
	case categoryAll:
		return "全部"
	case categoryImage:
		return "图片"
	case categoryVideo:
		return "视频"
	case categoryAudio:
		return "音频"
	case categoryLive:
		return "直播"
	case categorySegments:
		return "分段媒体"
	default:
		return string(value)
	}
}

// candidateCategory 返回候选在界面上的分类。直播与普通视频互斥：Live 候选只进
// “直播”和“全部”，不混入视频；有限 FLV（Live=false）按视频显示。未知文件只进“全部”。
func candidateCategory(candidate miniprogram.Candidate) category {
	if candidate.Live {
		return categoryLive
	}
	switch miniprogram.MediaKind(candidate) {
	case "m3u8":
		return categorySegments
	case "image":
		return categoryImage
	case "video":
		return categoryVideo
	case "audio":
		return categoryAudio
	default:
		return ""
	}
}

func kindDisplay(kind string) string {
	switch kind {
	case "image":
		return "图片"
	case "video":
		return "视频"
	case "audio":
		return "音频"
	case "m3u8":
		return "分段媒体"
	case "live":
		return "直播流"
	case "file":
		return "其他"
	default:
		if kind == "" {
			return "-"
		}
		return kind
	}
}

// candidateKindDisplay 在“类型”列和日志里显示媒体类型；疑似 HTTP-FLV 直播流单独
// 标为直播流，m3u8 分类改称分段媒体。
func candidateKindDisplay(candidate miniprogram.Candidate) string {
	if candidate.Live {
		return "直播流"
	}
	return kindDisplay(candidate.Kind)
}

func sourceDisplay(source string) string {
	switch source {
	case "response":
		return "响应"
	case "json":
		return "JSON"
	case "html":
		return "H5"
	case "-":
		return "-"
	default:
		return source
	}
}

// attributionMarker 在列表中标记来源可信度：✓ 来源已确认，? 来源未确认。
func attributionMarker(candidate miniprogram.Candidate) string {
	switch candidate.Attribution {
	case miniprogram.AttributionConfirmed:
		return "✓"
	case miniprogram.AttributionUnattributed:
		return "?"
	default:
		return "-"
	}
}

func attributionDisplay(candidate miniprogram.Candidate) string {
	switch candidate.Attribution {
	case miniprogram.AttributionConfirmed:
		return "来源已确认"
	case miniprogram.AttributionUnattributed:
		return "来源未确认"
	default:
		return "-"
	}
}

// candidateBasis 解释候选是如何被识别的，帮助用户判断未归属资源是否可信。
func candidateBasis(candidate miniprogram.Candidate) string {
	if candidate.Provisional {
		return "字段名推断（低置信）"
	}
	if candidate.Live {
		if candidate.Source == "response" {
			return "HTTP-FLV 直播流（响应识别，下载即持续录制）"
		}
		return "疑似 HTTP-FLV 直播流（URL/字段推断，下载时按实际响应确认）"
	}
	if miniprogram.MediaKind(candidate) == "m3u8" {
		return "m3u8/HLS 分段媒体（由播放列表描述）"
	}
	if candidate.Kind == "file" {
		return "未知二进制兜底（≥256KB）"
	}
	switch candidate.Source {
	case "html":
		return "H5 页面/脚本扫描（未播放验证）"
	case "json":
		if candidate.FieldPath != "" {
			return "JSON 字段 " + candidate.FieldPath
		}
		return "JSON 字段"
	case "response":
		if candidate.Attribution == miniprogram.AttributionUnattributed {
			return "响应类型/URL 推断，来源未知"
		}
		return "响应类型/URL 推断"
	default:
		return "-"
	}
}

func headerSummary(headers map[string]string) string {
	if len(headers) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(headers[key])
		switch strings.ToLower(key) {
		case "authorization", "cookie", "proxy-authorization":
			value = "已捕获"
		}
		if value == "" {
			value = "-"
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "; ")
}

func formatBytes(size int64) string {
	if size <= 0 {
		return "-"
	}
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value = value / unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fPB", value/unit)
}

func formatDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	totalSeconds := ms / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

var recordTimeRegexp = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z-(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2}):\d{2}Z`)

func candidateLabel(candidate miniprogram.Candidate) string {
	quality := ""
	if candidate.Live {
		quality = liveQualityLabel(candidate.URL)
	}
	if title := strings.TrimSpace(candidate.Title); title != "" {
		if quality != "" && !strings.Contains(title, quality) {
			return title + " · " + quality
		}
		return title
	}
	u, err := url.Parse(candidate.URL)
	if err != nil {
		return candidateKindDisplay(candidate)
	}
	base := path.Base(u.Path)
	if unescaped, err := url.PathUnescape(base); err == nil && unescaped != "" {
		base = unescaped
	}
	if candidate.Kind == "m3u8" {
		if match := recordTimeRegexp.FindStringSubmatch(base); len(match) == 5 {
			if match[1] == match[3] {
				return "回放 " + match[1] + " " + match[2] + "-" + match[4]
			}
			return "回放 " + match[1] + " " + match[2] + " 至 " + match[3] + " " + match[4]
		}
		if stream := streamName(u.Path); stream != "" {
			return "分段流 · " + shortenOpaqueID(stream)
		}
	}
	stem := ""
	if base != "" && base != "." && base != "/" {
		stem = strings.TrimSuffix(base, path.Ext(base))
	}
	if candidate.Live {
		parts := []string{"直播流"}
		if quality != "" {
			parts = append(parts, quality)
		}
		if stream := streamName(u.Path); stream != "" {
			parts = append(parts, shortenOpaqueID(stream))
		} else if stem != "" {
			parts = append(parts, shortenOpaqueID(stripQualitySuffix(stem)))
		} else if u.Hostname() != "" {
			parts = append(parts, u.Hostname())
		}
		return strings.Join(parts, " · ")
	}
	if stem != "" && !miniprogram.LooksLikeOpaqueID(stem) {
		return stem
	}
	return candidateFallbackSummary(candidate, u)
}

func candidateFallbackSummary(candidate miniprogram.Candidate, u *url.URL) string {
	label := fieldDisplay(candidate.FieldPath)
	if label == "" {
		ext := strings.TrimPrefix(strings.ToUpper(candidate.Suffix), ".")
		if ext == "" && u != nil {
			ext = strings.TrimPrefix(strings.ToUpper(path.Ext(u.Path)), ".")
		}
		kindName := candidateKindDisplay(candidate)
		if ext != "" && kindName != "-" && kindName != "其他" {
			label = ext + " " + kindName
		} else if kindName != "-" {
			label = kindName
		} else {
			label = "媒体"
		}
	}
	if domain, _ := miniprogram.ExtractBusinessDomain(candidate.URL); domain != "" {
		return label + " · " + domain
	}
	if u != nil && u.Hostname() != "" {
		return label + " · " + u.Hostname()
	}
	return label
}

func liveQualityLabel(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path)))
	switch {
	case strings.HasSuffix(lower, "_uhd"), strings.HasSuffix(lower, "_4k"), strings.HasSuffix(lower, "_2160p"):
		return "蓝光 UHD"
	case strings.HasSuffix(lower, "_d3fhd"), strings.HasSuffix(lower, "_efhd"), strings.HasSuffix(lower, "_fhd"), strings.HasSuffix(lower, "_1080p"):
		return "超清 FHD"
	case strings.HasSuffix(lower, "_ehd"), strings.HasSuffix(lower, "_hd"), strings.HasSuffix(lower, "_720p"):
		return "高清 HD"
	case strings.HasSuffix(lower, "_esd"), strings.HasSuffix(lower, "_sd"), strings.HasSuffix(lower, "_480p"):
		return "标清 SD"
	case strings.HasSuffix(lower, "_eld"), strings.HasSuffix(lower, "_ld"), strings.HasSuffix(lower, "_360p"):
		return "流畅 LD"
	case strings.HasSuffix(lower, "_orig"), strings.HasSuffix(lower, "_origin"):
		return "原画"
	}
	return ""
}

func stripQualitySuffix(stem string) string {
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

func shortenOpaqueID(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= 10 {
		return s
	}
	if miniprogram.LooksLikeOpaqueID(s) {
		return string(runes[:4]) + "…" + string(runes[len(runes)-4:])
	}
	return s
}

func candidateReadableInfo(candidate miniprogram.Candidate) string {
	u, err := url.Parse(candidate.URL)
	if err != nil {
		return candidate.URL
	}
	var parts []string
	if field := fieldDisplay(candidate.FieldPath); field != "" {
		parts = append(parts, field)
	}
	if u.Host != "" {
		parts = append(parts, u.Host)
	}
	base := path.Base(u.Path)
	if base != "" && base != "." && base != "/" {
		parts = append(parts, base)
	}
	if len(parts) == 0 {
		return candidate.URL
	}
	return strings.Join(parts, " · ")
}

func fieldDisplay(fieldPath string) string {
	key := strings.ToLower(fieldPath)
	switch {
	case strings.Contains(key, "cover"):
		return "封面"
	case strings.Contains(key, "poster"):
		return "海报"
	case strings.Contains(key, "avatar"):
		return "头像"
	case strings.Contains(key, "thumb"):
		return "缩略图"
	case strings.Contains(key, "banner"):
		return "横幅"
	case strings.Contains(key, "logo"):
		return "标志"
	case strings.Contains(key, "video"):
		return "视频"
	case strings.Contains(key, "audio"), strings.Contains(key, "voice"), strings.Contains(key, "sound"):
		return "音频"
	case strings.Contains(key, "image"), strings.Contains(key, "img"), strings.Contains(key, "pic"), strings.Contains(key, "photo"):
		return "图片"
	default:
		return ""
	}
}

func streamName(rawPath string) string {
	parts := strings.Split(rawPath, "/")
	for i, part := range parts {
		if part == "streams" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
