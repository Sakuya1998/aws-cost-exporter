package collector

import (
	"fmt"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

// BuildDailyAndMTDQueries constructs daily and month-to-date Cost Explorer queries.
func BuildDailyAndMTDQueries(reference time.Time, groupBy cost.DimensionKind) ([]ports.CostQuery, error) {
	day := cost.DayContaining(reference)
	month := cost.MonthContaining(reference)
	monthToDate, err := cost.NewPeriod(month.Start(), day.End())
	if err != nil {
		return nil, fmt.Errorf("build month-to-date period: %w", err)
	}
	return []ports.CostQuery{
		{Period: day, Window: cost.WindowDaily, GroupBy: groupBy},
		{Period: monthToDate, Window: cost.WindowMonthToDate, GroupBy: groupBy},
	}, nil
}
