package cur

import (
	"context"
	"fmt"
	"time"

	basecollector "github.com/sakuya1998/aws-cost-exporter/internal/collector"
	tagcollector "github.com/sakuya1998/aws-cost-exporter/internal/collector/tag"
	"github.com/sakuya1998/aws-cost-exporter/internal/config"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
)

type Reader interface {
	ReadCosts(context.Context, time.Time, []cost.Basis) ([]cost.Cost, error)
	ReadTagCosts(context.Context, time.Time, []cost.Basis) ([]tagcost.Cost, error)
}

const Name = "cur"

type Collector struct {
	target         identity.TargetID
	reader         Reader
	bases          []cost.Basis
	tags           bool
	seriesLimit    int
	tagSeriesLimit int
	keys           []config.TagKeyConfig
	overflow       string
	observers      []basecollector.OverflowObserver
}

func New(target identity.TargetID, reader Reader, bases []cost.Basis, tags bool, seriesLimit, tagSeriesLimit int, keys []config.TagKeyConfig, overflow string, observers ...basecollector.OverflowObserver) (*Collector, error) {
	if reader == nil || seriesLimit <= 0 || tagSeriesLimit <= 0 {
		return nil, fmt.Errorf("CUR reader must not be nil")
	}
	return &Collector{target: target, reader: reader, bases: append([]cost.Basis(nil), bases...), tags: tags, seriesLimit: seriesLimit, tagSeriesLimit: tagSeriesLimit, keys: append([]config.TagKeyConfig(nil), keys...), overflow: overflow, observers: append([]basecollector.OverflowObserver(nil), observers...)}, nil
}
func (collector *Collector) ID() identity.CollectorID {
	return identity.CollectorID{Target: collector.target, Name: Name}
}
func (collector *Collector) Collect(ctx context.Context, reference time.Time) (snapshot.PartialSnapshot, error) {
	costs, err := collector.reader.ReadCosts(ctx, reference, collector.bases)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	var tags []tagcost.Cost
	if collector.tags {
		tags, err = collector.reader.ReadTagCosts(ctx, reference, collector.bases)
		if err != nil {
			return snapshot.PartialSnapshot{}, err
		}
		tags, err = collector.limitTags(tags)
		if err != nil {
			return snapshot.PartialSnapshot{}, err
		}
		if len(tags) > collector.tagSeriesLimit {
			return snapshot.PartialSnapshot{}, fmt.Errorf("CUR tag series limit exceeded")
		}
	}
	if len(costs)+len(tags) > collector.seriesLimit {
		return snapshot.PartialSnapshot{}, fmt.Errorf("CUR series limit exceeded")
	}
	return snapshot.NewWithData(costs, nil, nil, nil, nil, nil, tags), nil
}

func (collector *Collector) limitTags(values []tagcost.Cost) ([]tagcost.Cost, error) {
	limits := make(map[string]int, len(collector.keys))
	for _, key := range collector.keys {
		limits[key.Key] = key.MaxValues
	}
	type groupKey struct {
		key      string
		provider cost.Provider
		basis    cost.Basis
		window   cost.Window
		currency string
	}
	groups := make(map[groupKey][]tagcost.Cost)
	for _, item := range values {
		limit, ok := limits[item.TagKey]
		if !ok || limit <= 0 {
			return nil, fmt.Errorf("CUR returned a non-allowlisted tag key")
		}
		key := groupKey{item.TagKey, item.Provider, item.Basis, item.Window, item.Amount.Currency()}
		groups[key] = append(groups[key], item)
	}
	result := make([]tagcost.Cost, 0, len(values))
	for key, group := range groups {
		bounded, err := tagcollector.LimitValues(group, limits[key.key], collector.overflow, collector.observers...)
		if err != nil {
			return nil, err
		}
		result = append(result, bounded...)
	}
	tagcost.Sort(result)
	return result, nil
}
