package ports

import (
	"context"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
)

type CommitmentReader interface {
	ReadSavingsPlansUtilization(context.Context, time.Time) (commitment.Summary, error)
	ReadSavingsPlansCoverage(context.Context, time.Time) (commitment.Summary, error)
	ReadReservationUtilization(context.Context, time.Time) (commitment.Summary, error)
	ReadReservationCoverage(context.Context, time.Time) (commitment.Summary, error)
}
