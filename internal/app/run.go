package app

import (
	"context"
	"log"
	"time"

	"forumdesk/internal/telegram"
)

type Poller interface {
	GetUpdates(context.Context, int) ([]telegram.Update, error)
}

type UpdateHandler interface {
	Handle(context.Context, telegram.Update) error
}

var retryDelay = 2 * time.Second

func Run(ctx context.Context, logger *log.Logger, poller Poller, handler UpdateHandler) {
	offset := 0
	for ctx.Err() == nil {
		updates, err := poller.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Printf("get updates: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := handler.Handle(ctx, update); err != nil {
				logger.Printf("update %d: %v", update.UpdateID, err)
			}
		}
	}
}
