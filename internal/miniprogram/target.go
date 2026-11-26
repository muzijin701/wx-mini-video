package miniprogram

import (
	"net"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Program 记录本次运行中观察到的小程序，只保留发现列表需要的元数据。
type Program struct {
	AppID     string    `json:"app_id"`
	Domain    string    `json:"domain,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// now 是测试注入点：发现列表的首次/最近出现时间统一从这里获取。
var now = time.Now

// SetTarget 切换目标：清空目标候选并让在途旧响应失效，但按 ADR 0008 保留
// 与目标无关的未归属列表，避免用户切换目标后丢失 H5 视频。
func (s *Store) SetTarget(target Target) {
	target.AppID = strings.TrimSpace(target.AppID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.target = target
	s.generation++
	s.items = make(map[string]Candidate)
	s.order = nil
}

func (s *Store) IsCurrentTarget(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.generation == generation && s.target.AppID != ""
}

func (s *Store) Observe(appID string) (Target, uint64) {
	return s.ObserveRequest(appID, "")
}

// ObserveRequest 登记请求来源 AppID 并统计其请求的业务域名提示。
func (s *Store) ObserveRequest(appID, rawURL string) (Target, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observe(appID)
	if appID != "" && rawURL != "" {
		if domain, _ := ExtractBusinessDomain(rawURL); domain != "" {
			if s.programHosts == nil {
				s.programHosts = make(map[string]map[string]int)
			}
			counts := s.programHosts[appID]
			if counts == nil {
				counts = make(map[string]int)
				s.programHosts[appID] = counts
			}
			counts[domain]++
		}
	}
	return s.target, s.generation
}

// observe 更新发现列表：已有条目只刷新最近出现时间，新条目首次出现与最近出现时间相同。
func (s *Store) observe(appID string) {
	if appID == "" {
		return
	}
	observedAt := now()
	for i := range s.programs {
		if s.programs[i].AppID == appID {
			s.programs[i].LastSeen = observedAt
			return
		}
	}
	s.programs = append(s.programs, Program{AppID: appID, FirstSeen: observedAt, LastSeen: observedAt})
}

// Programs 返回发现列表的副本：最近出现的排在前面；最近出现相同的按首次出现升序。
func (s *Store) Programs() []Program {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]Program(nil), s.programs...)
	for i := range out {
		out[i].Domain = s.bestProgramDomainLocked(out[i].AppID)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].FirstSeen.Before(out[j].FirstSeen)
	})
	return out
}

// ProgramDomain 返回指定 AppID 统计出的业务域名提示（仅供界面辅助辨识）。
func (s *Store) ProgramDomain(appID string) string {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bestProgramDomainLocked(appID)
}

func (s *Store) bestProgramDomainLocked(appID string) string {
	counts := s.programHosts[appID]
	if len(counts) == 0 {
		return ""
	}
	bestBusiness, bestBusinessCount := "", 0
	bestGeneric, bestGenericCount := "", 0
	for domain, count := range counts {
		if isGenericDomain(domain) {
			if count > bestGenericCount || (count == bestGenericCount && (bestGeneric == "" || domain < bestGeneric)) {
				bestGeneric = domain
				bestGenericCount = count
			}
			continue
		}
		if count > bestBusinessCount || (count == bestBusinessCount && (bestBusiness == "" || domain < bestBusiness)) {
			bestBusiness = domain
			bestBusinessCount = count
		}
	}
	if bestBusiness != "" {
		return bestBusiness
	}
	return bestGeneric
}

// ExtractBusinessDomain 从请求 URL 提取归一化后的可注册域名，并标记是否为微信/通用基础云域。
func ExtractBusinessDomain(rawURL string) (domain string, generic bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(u.Hostname()), "."))
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return "", false
	}
	if host == "servicewechat.com" || strings.HasSuffix(host, ".servicewechat.com") {
		return "", false
	}
	domain = registrableDomain(host)
	if domain == "" {
		return "", false
	}
	return domain, isGenericDomain(domain)
}

func registrableDomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return ""
	}
	if len(parts) == 2 {
		return host
	}
	tail2 := parts[len(parts)-2] + "." + parts[len(parts)-1]
	switch tail2 {
	case "com.cn", "net.cn", "org.cn", "gov.cn", "edu.cn", "co.uk", "com.hk", "com.tw", "com.sg", "co.jp":
		if len(parts) >= 3 {
			return parts[len(parts)-3] + "." + tail2
		}
	}
	return tail2
}

func isGenericDomain(domain string) bool {
	switch domain {
	case "qq.com", "qpic.cn", "qlogo.cn", "wechat.com", "myqcloud.com", "qcloud.com",
		"aliyuncs.com", "volces.com", "byteimg.com", "baidubce.com", "bcebos.com",
		"cdnhwc1.com", "cdnhwc2.com":
		return true
	}
	return false
}
