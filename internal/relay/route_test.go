package relay

import (
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

func newTestGroup(id int, mode model.GroupMode, items ...model.GroupItem) model.Group {
	for i := range items {
		items[i].GroupID = id
		if items[i].ID == 0 {
			items[i].ID = i + 1
		}
		if items[i].Weight == 0 {
			items[i].Weight = 1
		}
		items[i].Priority = i + 1
	}
	return model.Group{
		ID:    id,
		Mode:  mode,
		Items: items,
		RelayConfig: model.GroupRelayConfig{
			MemberMaxAttempts:     2,
			MemberCooldownSeconds: 60,
			MemberAffinitySeconds: 300,
		},
	}
}

func TestPickGroupItemManual(t *testing.T) {
	group := newTestGroup(101, model.GroupModeManual, model.GroupItem{}, model.GroupItem{})
	t.Cleanup(func() { ResetRouteState(group.ID) })

	group.ActiveItemID = group.Items[1].ID
	if got := pickGroupItem(group); got.ID != group.Items[1].ID {
		t.Fatalf("manual pick = %d, want %d", got.ID, group.Items[1].ID)
	}

	group.ActiveItemID = 0
	if got := pickGroupItem(group); got.ID != 0 {
		t.Fatalf("manual pick without active = %d, want 0", got.ID)
	}
}

func TestPickGroupItemFailoverPriorityAndCooldown(t *testing.T) {
	group := newTestGroup(102, model.GroupModeFailover, model.GroupItem{}, model.GroupItem{})
	t.Cleanup(func() { ResetRouteState(group.ID) })

	first, second := group.Items[0], group.Items[1]
	if got := pickGroupItem(group); got.ID != first.ID {
		t.Fatalf("first pick = %d, want %d", got.ID, first.ID)
	}
	if cooled := recordRouteFailure(group, first.ID, 1); cooled {
		t.Fatalf("failure below max attempts must not cool down")
	}
	if cooled := recordRouteFailure(group, first.ID, 2); !cooled {
		t.Fatalf("failure at max attempts must cool down")
	}
	if got := pickGroupItem(group); got.ID != second.ID {
		t.Fatalf("pick after cooldown = %d, want %d", got.ID, second.ID)
	}

	// 亲和期内沿用当前成员, 不回到已恢复的高优先级成员。
	recordRouteSuccess(group, second.ID)
	expireCooldown(group.ID, first.ID)
	if got := pickGroupItem(group); got.ID != second.ID {
		t.Fatalf("pick during affinity = %d, want %d", got.ID, second.ID)
	}

	// 亲和结束且冷却到期后, 只放行一个探测请求回到高优先级成员。
	routeMu.Lock()
	route := routes[group.ID]
	route.AffinityUntil = 0
	routeMu.Unlock()
	if got := pickGroupItem(group); got.ID != first.ID {
		t.Fatalf("probe pick = %d, want %d", got.ID, first.ID)
	}
	if got := pickGroupItem(group); got.ID != second.ID {
		t.Fatalf("pick while probe busy = %d, want %d", got.ID, second.ID)
	}
}

func TestPickGroupItemRoundRobin(t *testing.T) {
	group := newTestGroup(103, model.GroupModeRoundRobin, model.GroupItem{}, model.GroupItem{}, model.GroupItem{})
	t.Cleanup(func() { ResetRouteState(group.ID) })

	want := []int{group.Items[0].ID, group.Items[1].ID, group.Items[2].ID, group.Items[0].ID, group.Items[1].ID}
	for i, wantID := range want {
		if got := pickGroupItem(group); got.ID != wantID {
			t.Fatalf("pick %d = %d, want %d", i, got.ID, wantID)
		}
	}
}

func TestPickGroupItemRandom(t *testing.T) {
	group := newTestGroup(104, model.GroupModeRandom, model.GroupItem{}, model.GroupItem{}, model.GroupItem{})
	t.Cleanup(func() { ResetRouteState(group.ID) })

	valid := make(map[int]bool, len(group.Items))
	for _, item := range group.Items {
		valid[item.ID] = true
	}
	seen := make(map[int]bool, len(group.Items))
	for i := 0; i < 100; i++ {
		got := pickGroupItem(group)
		if !valid[got.ID] {
			t.Fatalf("random pick returned unknown item %d", got.ID)
		}
		seen[got.ID] = true
	}
	if len(seen) != len(group.Items) {
		t.Fatalf("random pick only used %d of %d items in 100 rounds", len(seen), len(group.Items))
	}
}

func TestPickGroupItemWeighted(t *testing.T) {
	group := newTestGroup(105, model.GroupModeWeighted, model.GroupItem{Weight: 9}, model.GroupItem{Weight: 1})
	t.Cleanup(func() { ResetRouteState(group.ID) })

	heavy := 0
	const rounds = 400
	for i := 0; i < rounds; i++ {
		if pickGroupItem(group).ID == group.Items[0].ID {
			heavy++
		}
	}
	// 权重 9:1 下重权成员作为首选的期望约 90%, 阈值放宽到 70% 以避免随机波动导致偶发失败。
	if heavy < rounds*7/10 {
		t.Fatalf("weighted heavy picks = %d/%d, want >= %d", heavy, rounds, rounds*7/10)
	}
}

func TestWeightedOrderZeroWeightNormalized(t *testing.T) {
	ordered := weightedOrder([]model.GroupItem{{ID: 1, Weight: 0}, {ID: 2, Weight: 0}})
	if len(ordered) != 2 {
		t.Fatalf("ordered length = %d, want 2", len(ordered))
	}
	if ordered[0].ID != 1 && ordered[0].ID != 2 {
		t.Fatalf("unexpected first item %d", ordered[0].ID)
	}
}

// expireCooldown 把指定成员的冷却截止时间调到过去, 便于在不等待真实冷却时长的前提下测试探测与恢复路径。
func expireCooldown(groupID, itemID int) {
	routeMu.Lock()
	defer routeMu.Unlock()
	if route := routes[groupID]; route != nil {
		route.Cooldowns[itemID] = time.Now().Add(-time.Second).UnixMilli()
	}
}
