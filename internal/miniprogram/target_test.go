package miniprogram

import (
	"testing"
	"time"
)

// stubProgramClock 替换包级 now，按顺序返回给定时间；用尽后返回最后一个。
func stubProgramClock(t *testing.T, times ...time.Time) {
	t.Helper()
	old := now
	index := 0
	now = func() time.Time {
		if index >= len(times) {
			return times[len(times)-1]
		}
		value := times[index]
		index++
		return value
	}
	t.Cleanup(func() { now = old })
}

func TestProgramsTrackFirstAndLastSeen(t *testing.T) {
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	stubProgramClock(t, base, base.Add(10*time.Second), base.Add(20*time.Second), base.Add(30*time.Second))
	store := NewStore()

	store.Observe("wx-a") // first=base last=base
	store.Observe("wx-b") // first=+10s last=+10s
	store.Observe("wx-b") // last=+20s
	store.Observe("wx-a") // last=+30s

	programs := store.Programs()
	if len(programs) != 2 {
		t.Fatalf("len(Programs()) = %d, want 2", len(programs))
	}
	if programs[0].AppID != "wx-a" || programs[1].AppID != "wx-b" {
		t.Fatalf("Programs() = %#v, want wx-a then wx-b ordered by last seen", programs)
	}
	if !programs[0].FirstSeen.Equal(base) || !programs[0].LastSeen.Equal(base.Add(30*time.Second)) {
		t.Fatalf("wx-a times = %#v, want first=base last=+30s", programs[0])
	}
	if !programs[1].FirstSeen.Equal(base.Add(10*time.Second)) || !programs[1].LastSeen.Equal(base.Add(20*time.Second)) {
		t.Fatalf("wx-b times = %#v, want first=+10s last=+20s", programs[1])
	}
}

func TestProgramsSortByLastSeenThenFirstSeen(t *testing.T) {
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	// p 首次 +1s 最近 +4s；q 首次 +2s 最近 +4s；r 首次 +3s 最近 +3s。
	stubProgramClock(t, base.Add(time.Second), base.Add(2*time.Second), base.Add(4*time.Second), base.Add(4*time.Second), base.Add(3*time.Second))
	store := NewStore()
	store.Observe("p")
	store.Observe("q")
	store.Observe("p")
	store.Observe("q")
	store.Observe("r")

	programs := store.Programs()
	got := make([]string, len(programs))
	for i, program := range programs {
		got[i] = program.AppID
	}
	want := []string{"p", "q", "r"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Programs() order = %v, want %v", got, want)
		}
	}
}

func TestProgramsReturnsCopy(t *testing.T) {
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	stubProgramClock(t, base)
	store := NewStore()
	store.Observe("wx-a")

	programs := store.Programs()
	programs[0].AppID = "tampered"
	if got := store.Programs(); len(got) != 1 || got[0].AppID != "wx-a" {
		t.Fatalf("Programs() exposed internal state: %#v", got)
	}
}

func TestObserveIgnoresEmptyAppID(t *testing.T) {
	store := NewStore()
	store.Observe("")
	if got := store.Programs(); len(got) != 0 {
		t.Fatalf("Programs() = %#v, want no entry for empty AppID", got)
	}
}

func TestObserveRequestExtractsBusinessDomainHint(t *testing.T) {
	store := NewStore()
	// 通用微信域请求更多，但只要存在业务域名，优先显示真实业务域名。
	for range 5 {
		store.ObserveRequest("wx123", "https://res.wx.qq.com/mmbiz/a.png")
	}
	store.ObserveRequest("wx123", "https://v3.eastbuy.com/vod/demo.mp4")
	store.ObserveRequest("wx123", "https://live.eastbuy.com/stream.flv")

	if got := store.ProgramDomain("wx123"); got != "eastbuy.com" {
		t.Fatalf("ProgramDomain(wx123) = %q, want eastbuy.com", got)
	}
	programs := store.Programs()
	if len(programs) != 1 || programs[0].Domain != "eastbuy.com" {
		t.Fatalf("Programs() = %#v, want domain eastbuy.com", programs)
	}

	// 仅出现通用 CDN 域时兜底显示该域，不显示 servicewechat.com 或 IP。
	store.ObserveRequest("wx456", "https://127.0.0.1:8080/local.mp4")
	store.ObserveRequest("wx456", "https://servicewechat.com/wx456/1/page-frame.html")
	store.ObserveRequest("wx456", "https://wx.qlogo.cn/mmhead/avatar.jpg")
	if got := store.ProgramDomain("wx456"); got != "qlogo.cn" {
		t.Fatalf("ProgramDomain(wx456) = %q, want qlogo.cn", got)
	}

	// 多段国家域名保留完整可注册域。
	store.ObserveRequest("wx789", "https://vod.example.com.cn/lesson.mp4")
	if got := store.ProgramDomain("wx789"); got != "example.com.cn" {
		t.Fatalf("ProgramDomain(wx789) = %q, want example.com.cn", got)
	}
}
