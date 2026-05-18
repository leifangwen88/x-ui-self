package service

import "sort"

// syncEventPriority 数值越小越优先（游戏 → SOCKS → 入站 → 其它）
func syncEventPriority(typ string) int {
	switch typ {
	case SyncEventClusterMemberUpsert, SyncEventClusterMemberRemove:
		return 5
	case SyncEventGameUpsert, SyncEventGameDelete:
		return 10
	case SyncEventSocksUpsert, SyncEventSocksDelete:
		return 20
	case SyncEventInboundUpsert, SyncEventInboundDelete,
		SyncEventInboundBindSocks, SyncEventInboundBindGame, SyncEventInboundRemark:
		return 30
	case SyncEventMarkUsed, SyncEventMarkBanned, SyncEventMarkClear, SyncEventMarkUnban:
		return 80
	case SyncEventXrayTemplate:
		return 90
	default:
		return 50
	}
}

func sortSyncEventsByPriority(events []SyncEventDTO) {
	sort.Slice(events, func(i, j int) bool {
		pi := syncEventPriority(events[i].Type)
		pj := syncEventPriority(events[j].Type)
		if pi != pj {
			return pi < pj
		}
		return events[i].CreatedAt < events[j].CreatedAt
	})
}
