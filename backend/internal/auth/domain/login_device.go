package domain

import (
	"sort"
	"strings"
	"time"
)

// LoginDevice 是"登录设备"列表的一行：同一设备的多条会话聚合后的视图。
//
// 与 Session 的区别：Session 是一次登录的服务端记录（可以有多条），
// LoginDevice 是用户眼里的一台设备（一台一条）。它不落库，只在读路径上
// 由会话列表实时聚合出来——零 DDL，也不引入"设备表"这种第二真相。
type LoginDevice struct {
	// SessionID 是该设备的代表会话：若当前会话属于本设备，就是当前会话，
	// 否则取最近一次登录的会话。「当前设备」标记靠它与会话 ID 比对，
	// 不靠设备描述，否则同设备的其它会话会被误标。
	SessionID SessionID

	// Device 是代表会话 UA 的解析结果。
	Device Device

	// IP / Addr / UA 取代表会话（通常就是最近一次登录）的客户端信息。
	IP   string
	Addr string
	UA   string

	// FirstSeenAt 是该设备在本账号下最早一次登录时间。
	FirstSeenAt time.Time
	// LastSeenAt 是该设备最近一次活跃时间。
	LastSeenAt time.Time
	// ExpiresAt 是代表会话的过期时间。
	ExpiresAt time.Time
	// LoginCount 是聚合进本设备的会话条数（即累计登录次数）。
	LoginCount int
}

// AggregateDevices 把某用户的有效会话按设备聚合。
//
// 分组键是 UA 解析后的设备描述（浏览器 + 系统 + 类型），版本号不参与，
// 因此同一设备升级浏览器不会被拆成两条；解析不出的 UA 退回原文分组，
// 保证"不同 UA 各一条"，也不会把两个陌生客户端揉成一条。
//
// current 是当前请求所用会话 ID：它所属的设备把代表会话设为它，
// 使「下线当前设备」能吊销到真正在用的那条会话。
//
// 返回结果按最近活跃倒序；时间相同时按首次登录、再按会话 ID 兜底，
// 保证顺序确定（内存仓储的遍历顺序是随机的）。
func AggregateDevices(sessions []Session, current SessionID) []LoginDevice {
	groups := make(map[string]*deviceGroup, len(sessions))
	for _, s := range sessions {
		key := DeviceGroupKey(s)
		g, ok := groups[key]
		if !ok {
			g = &deviceGroup{firstSeen: s.CreatedAt}
			groups[key] = g
		}
		g.add(s, current)
	}

	out := make([]LoginDevice, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.build())
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return out[i].LastSeenAt.After(out[j].LastSeenAt)
		}
		if !out[i].FirstSeenAt.Equal(out[j].FirstSeenAt) {
			return out[i].FirstSeenAt.After(out[j].FirstSeenAt)
		}
		return out[i].SessionID.Uint64() > out[j].SessionID.Uint64()
	})
	return out
}

// DeviceGroupKey 返回一条会话所属设备的分组键。
//
// 解析出的设备描述优先（浏览器 + 系统 + 类型，忽略版本号）；
// 解析不出时退回 UA 原文，保证"不同 UA 各一条"且两个陌生客户端不会被揉在一起。
// 下发"下线设备"时也用它：同键的会话都算同一台设备。
func DeviceGroupKey(s Session) string {
	if key := ParseUserAgent(s.UA).Key(); key != "" {
		return "device:" + key
	}
	return "ua:" + strings.ToLower(strings.TrimSpace(s.UA))
}

// deviceGroup 是聚合过程中的可变累加器。
type deviceGroup struct {
	rep        Session
	firstSeen  time.Time
	lastSeen   time.Time
	count      int
	hasCurrent bool
}

func (g *deviceGroup) add(s Session, current SessionID) {
	if g.count == 0 {
		g.rep = s
	} else if !s.CreatedAt.IsZero() && (g.firstSeen.IsZero() || s.CreatedAt.Before(g.firstSeen)) {
		g.firstSeen = s.CreatedAt
	}

	if at := activityAt(s); !at.IsZero() && at.After(g.lastSeen) {
		g.lastSeen = at
	}
	g.count++

	if s.ID == current {
		// 当前会话优先当代表，哪怕它不是最近一次登录的那条。
		g.rep = s
		g.hasCurrent = true
		return
	}
	if g.hasCurrent {
		return
	}
	if s.CreatedAt.After(g.rep.CreatedAt) ||
		(s.CreatedAt.Equal(g.rep.CreatedAt) && s.ID.Uint64() > g.rep.ID.Uint64()) {
		g.rep = s
	}
}

func (g *deviceGroup) build() LoginDevice {
	return LoginDevice{
		SessionID:   g.rep.ID,
		Device:      ParseUserAgent(g.rep.UA),
		IP:          g.rep.IP,
		Addr:        g.rep.Addr,
		UA:          g.rep.UA,
		FirstSeenAt: g.firstSeen,
		LastSeenAt:  g.lastSeen,
		ExpiresAt:   g.rep.ExpiresAt,
		LoginCount:  g.count,
	}
}

// activityAt 取一条会话的"最近活跃时间"：优先 last_seen_at，缺失时退回创建时间。
func activityAt(s Session) time.Time {
	if s.LastSeenAt != nil && !s.LastSeenAt.IsZero() {
		return *s.LastSeenAt
	}
	return s.CreatedAt
}
