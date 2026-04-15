package service

import (
	"context"

	"xata/internal/o11y"
)

func RunGenericService(ctx context.Context, o *o11y.O, svc RunnerService) error {
	return svc.Run(ctx, o)
}
