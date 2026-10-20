package server

import (
	"context"
	"database/sql"
	"fmt"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-inventory/internal/store"
)

func (s *Server) itemEvent(ctx context.Context, actor *store.User, topic string, it *store.Item, extra map[string]any) core.CommitHook {
	if !s.core.Configured() {
		return nil
	}
	return func(tx *sql.Tx) error {
		payload := map[string]any{"item_id": it.ID, "item": it.Name, "quantity": it.Quantity, "unit": it.Unit}
		if topic == "inventory.item_created" {
			payload["category_id"] = it.CategoryID
			payload["location_id"] = it.LocationID
			payload["tags"] = it.Tags
		}
		if actor != nil {
			payload["user"] = actor.Email
		}
		for k, v := range extra {
			payload[k] = v
		}
		return core.QueueEvent(ctx, tx, topic, fmt.Sprintf("item:%d", it.ID), payload)
	}
}
