package commitment

import (
	"context"
	"fmt"
	"time"

	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/commitment"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

const Name = "commitments"

type Collector struct {
	target      identity.TargetID
	reader      ports.CommitmentReader
	seriesLimit int
}

func New(target identity.TargetID, reader ports.CommitmentReader, seriesLimit int) (*Collector, error) {
	if reader == nil {
		return nil, fmt.Errorf("commitment reader must not be nil")
	}
	if seriesLimit <= 0 {
		return nil, fmt.Errorf("commitment series limit must be positive")
	}
	return &Collector{target: target, reader: reader, seriesLimit: seriesLimit}, nil
}
func (collector *Collector) ID() identity.CollectorID {
	return identity.CollectorID{Target: collector.target, Name: Name}
}
func (collector *Collector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	spUtilization, err := collector.reader.ReadSavingsPlansUtilization(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	spCoverage, err := collector.reader.ReadSavingsPlansCoverage(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	sp, err := mergeSummaries(collector.target, domain.TypeSavingsPlan, spUtilization, spCoverage)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	riUtilization, err := collector.reader.ReadReservationUtilization(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	riCoverage, err := collector.reader.ReadReservationCoverage(ctx, reference)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	ri, err := mergeSummaries(collector.target, domain.TypeReservation, riUtilization, riCoverage)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	result := snapshot.NewWithData(nil, nil, nil, nil, []domain.Summary{sp, ri}, nil, nil)
	if result.SeriesCount() > collector.seriesLimit {
		return snapshot.PartialSnapshot{}, fmt.Errorf("commitment series limit exceeded")
	}
	return result, nil
}

func mergeSummaries(target identity.TargetID, commitmentType domain.Type, utilization, coverage domain.Summary) (domain.Summary, error) {
	if utilization.Target != target || coverage.Target != target || utilization.Type != commitmentType || coverage.Type != commitmentType ||
		utilization.TimeUnit == "" || utilization.TimeUnit != coverage.TimeUnit {
		return domain.Summary{}, fmt.Errorf("commitment reader returned inconsistent summary identity")
	}
	return domain.Summary{
		Target: target, Type: commitmentType, TimeUnit: utilization.TimeUnit,
		UtilizationRatio: utilization.UtilizationRatio, HasUtilization: utilization.HasUtilization,
		UnusedHours: utilization.UnusedHours, HasUnusedHours: utilization.HasUnusedHours,
		NetSavings: utilization.NetSavings, HasNetSavings: utilization.HasNetSavings,
		CoverageRatio: coverage.CoverageRatio, HasCoverage: coverage.HasCoverage,
		CoveredSpend: coverage.CoveredSpend, HasCoveredSpend: coverage.HasCoveredSpend,
		OnDemandCost: coverage.OnDemandCost, HasOnDemandCost: coverage.HasOnDemandCost,
	}, nil
}
