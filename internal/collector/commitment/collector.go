package commitment

import (
	"context"
	"fmt"
	"time"

	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

type Reader interface {
	ReadSavingsPlans(context.Context, time.Time) (domain.Summary, error)
	ReadReservations(context.Context, time.Time) (domain.Summary, error)
}

const Name = "commitments"

type Collector struct {
	target      identity.TargetID
	reader      Reader
	seriesLimit int
}

func New(target identity.TargetID, reader Reader, seriesLimit int) (*Collector, error) {
	if reader == nil || seriesLimit <= 0 {
		return nil, fmt.Errorf("commitment reader must not be nil")
	}
	return &Collector{target: target, reader: reader, seriesLimit: seriesLimit}, nil
}
func (collector *Collector) ID() identity.CollectorID {
	return identity.CollectorID{Target: collector.target, Name: Name}
}
func (collector *Collector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	sp, err := collector.reader.ReadSavingsPlans(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	ri, err := collector.reader.ReadReservations(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	result := snapshot.NewWithData(nil, nil, nil, nil, []domain.Summary{sp, ri}, nil, nil)
	if result.SeriesCount() > collector.seriesLimit {
		return snapshot.PartialSnapshot{}, fmt.Errorf("commitment series limit exceeded")
	}
	return result, nil
}
