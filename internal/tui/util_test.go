package tui

import (
	"testing"

	"github.com/MuZiJin701/wx-mini-video/internal/miniprogram"
)

func TestCandidateLabelTitlesLiveQualityAndFallbackSummary(t *testing.T) {
	// 1. 真实标题优先；直播附带清晰度标签。
	titledVideo := miniprogram.Candidate{
		Title: "商品讲解视频",
		Kind:  "video",
		URL:   "https://v3.eastbuy.com/vod/d8fb84bccd044dd6885dfd56332a8292.mp4",
	}
	if got := candidateLabel(titledVideo); got != "商品讲解视频" {
		t.Fatalf("titled video label = %q", got)
	}

	titledLive := miniprogram.Candidate{
		Title: "甄选生鲜专场",
		Kind:  "video",
		Live:  true,
		URL:   "https://live.eastbuy.com/7559179538913600_d3fhd.flv",
	}
	if got := candidateLabel(titledLive); got != "甄选生鲜专场 · 超清 FHD" {
		t.Fatalf("titled live label = %q", got)
	}

	// 2. 无标题直播显示清晰度与缩短流 ID。
	untitledLive := miniprogram.Candidate{
		Kind: "video",
		Live: true,
		URL:  "https://live.eastbuy.com/7559179538913600_ehd.flv",
	}
	if got := candidateLabel(untitledLive); got != "直播流 · 高清 HD · 7559…3600" {
		t.Fatalf("untitled live label = %q", got)
	}

	// 3. URL 中文文件名自动解码展示。
	encodedName := miniprogram.Candidate{
		Kind: "audio",
		URL:  "https://cdn.example.com/%E8%8B%B1%E8%AF%AD%E5%90%AC%E5%8A%9B01.mp3",
	}
	if got := candidateLabel(encodedName); got != "英语听力01" {
		t.Fatalf("decoded basename label = %q", got)
	}

	// 4. 纯哈希/纯数字路径改为语义兜底摘要，不再铺满十六进制哈希。
	opaqueCover := miniprogram.Candidate{
		Kind:      "image",
		Suffix:    ".jpg",
		FieldPath: "data.content[].coverUrl",
		URL:       "https://img.eastbuy.com/640",
	}
	if got := candidateLabel(opaqueCover); got != "封面 · eastbuy.com" {
		t.Fatalf("opaque cover summary = %q", got)
	}

	opaqueVideo := miniprogram.Candidate{
		Kind:   "video",
		Suffix: ".mp4",
		URL:    "https://v3.eastbuy.com/vod/af8f312f05994fd6bcd87c419b312b62-e89964c820f523ee7a2a.mp4",
	}
	if got := candidateLabel(opaqueVideo); got != "MP4 视频 · eastbuy.com" {
		t.Fatalf("opaque video summary = %q", got)
	}
}
