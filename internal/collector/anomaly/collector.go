package anomaly

import (
	"context"
	"fmt"
	"time"

	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/anomaly"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

type Reader interface {
	Read(context.Context, time.Time) (domain.Summary, error)
}

const Name = "anomalies"

type Collector struct {
	target      identity.TargetID
	reader      Reader
	seriesLimit int
}

func New(target identity.TargetID, reader Reader, seriesLimit int) (*Collector, error) {
	if reader == nil || seriesLimit <= 0 {
		return nil, fmt.Errorf("anomaly reader must not be nil")
	}
	return &Collector{target: target, reader: reader, seriesLimit: seriesLimit}, nil
}
func (collector *Collector) ID() identity.CollectorID {
	return identity.CollectorID{Target: collector.target, Name: Name}
}
func (collector *Collector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	value, err := collector.reader.Read(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	result := snapshot.NewWithData(nil, nil, nil, nil, nil, []domain.Summary{value}, nil)
	if result.SeriesCount() > collector.seriesLimit {
		return snapshot.PartialSnapshot{}, fmt.Errorf("anomaly series limit exceeded")
	}
	return result, nil
}
