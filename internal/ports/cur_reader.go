package ports

import (
	"context"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
)

type CURReader interface {
	QueryCosts(context.Context, time.Time, []cost.Basis) ([]cost.Cost, error)
	QueryTagCosts(context.Context, time.Time, []cost.Basis) ([]tagcost.Cost, error)
}
