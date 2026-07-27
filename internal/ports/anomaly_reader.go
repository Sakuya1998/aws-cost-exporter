package ports

import (
	"context"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
)

type AnomalyReader interface {
	ReadAnomalySummary(context.Context, time.Time) (anomaly.Summary, error)
}
