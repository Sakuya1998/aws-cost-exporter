package collector_test

import (
	"testing"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
)

func TestBuildDailyAndMTDQueries(t *testing.T) {
	reference := time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC)
	queries, err := basecollector.BuildDailyAndMTDQueries(reference, cost.DimensionService)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 {
		t.Fatalf("queries len = %d, want 2", len(queries))
	}
	if queries[0].Window != cost.WindowDaily || queries[0].GroupBy != cost.DimensionService {
		t.Fatalf("daily query = %#v", queries[0])
	}
	if queries[1].Window != cost.WindowMonthToDate {
		t.Fatalf("mtd query = %#v", queries[1])
	}
}
