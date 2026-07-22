package tag

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/collector"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

const Name = "tags"

type Reader interface {
	ReadTagCosts(context.Context, ports.CostQuery, string) ([]tagcost.Cost, error)
}
type Collector struct {
	target      identity.TargetID
	reader      Reader
	bases       []cost.Basis
	keys        []config.TagKeyConfig
	seriesLimit int
	overflow    string
	observers   []collector.OverflowObserver
}

func New(target identity.TargetID, reader Reader, bases []cost.Basis, keys []config.TagKeyConfig, seriesLimit int, overflow string, observers ...collector.OverflowObserver) (*Collector, error) {
	if reader == nil || len(keys) == 0 || seriesLimit <= 0 {
		return nil, fmt.Errorf("invalid tag collector")
	}
	return &Collector{target: target, reader: reader, bases: append([]cost.Basis(nil), bases...), keys: append([]config.TagKeyConfig(nil), keys...), seriesLimit: seriesLimit, overflow: overflow, observers: append([]collector.OverflowObserver(nil), observers...)}, nil
}
func (value *Collector) ID() identity.CollectorID {
	return identity.CollectorID{Target: value.target, Name: Name}
}
func (value *Collector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	queries, err := collector.BuildDailyAndMTDQueries(reference, cost.DimensionTotal)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	var result []tagcost.Cost
	for _, basis := range value.bases {
		for _, key := range value.keys {
			for _, query := range queries {
				query.Basis = basis
				rows, readErr := value.reader.ReadTagCosts(ctx, query, key.Key)
				if readErr != nil {
					return snapshot.PartialSnapshot{}, readErr
				}
				bounded, limitErr := LimitValues(rows, key.MaxValues, value.overflow, value.observers...)
				if limitErr != nil {
					return snapshot.PartialSnapshot{}, limitErr
				}
				for index := range bounded {
					bounded[index].Target = value.target
				}
				result = append(result, bounded...)
			}
		}
	}
	if len(result) > value.seriesLimit {
		return snapshot.PartialSnapshot{}, fmt.Errorf("tag series limit exceeded")
	}
	return snapshot.NewWithData(nil, nil, nil, nil, nil, nil, result), nil
}

// LimitValues keeps the largest tag values and aggregates overflow while
// preserving currency, provider, basis, window, and tag-key boundaries.
func LimitValues(values []tagcost.Cost, limit int, overflow string, observers ...collector.OverflowObserver) ([]tagcost.Cost, error) {
	if len(values) <= limit {
		return append([]tagcost.Cost(nil), values...), nil
	}
	ranked := append([]tagcost.Cost(nil), values...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Amount.Amount() != ranked[j].Amount.Amount() {
			return ranked[i].Amount.Amount() > ranked[j].Amount.Amount()
		}
		return ranked[i].TagValue < ranked[j].TagValue
	})
	keep := limit - 1
	if keep < 0 {
		return nil, fmt.Errorf("tag max values must be positive")
	}
	current := ranked[keep]
	total := 0.0
	for _, item := range ranked[keep:] {
		if item.Amount.Currency() != current.Amount.Currency() {
			return nil, fmt.Errorf("tag values contain mixed currencies")
		}
		if item.Provider != current.Provider || item.Basis != current.Basis || item.Window != current.Window || item.TagKey != current.TagKey {
			return nil, fmt.Errorf("tag values cross aggregation boundaries")
		}
		total += item.Amount.Amount()
	}
	amount, err := cost.NewMoney(total, current.Amount.Currency())
	if err != nil {
		return nil, err
	}
	current.TagValue, current.Amount = overflow, amount
	for _, observer := range observers {
		if observer != nil {
			observer.ObserveOverflow("tag", len(values)-keep)
		}
	}
	return append(ranked[:keep:keep], current), nil
}
