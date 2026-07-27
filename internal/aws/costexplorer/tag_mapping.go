package costexplorer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/tagcost"
	"github.com/sakuya1998/aws-cost-exporter/internal/ports"
)

func mapTagUsage(results []cetypes.ResultByTime, query ports.CostQuery, metric, tagKey string) ([]tagcost.Cost, error) {
	if !query.Basis.Valid() {
		return nil, fmt.Errorf("%w: unsupported tag cost basis", ErrInvalidResponse)
	}
	values := make([]tagcost.Cost, 0)
	for resultIndex, result := range results {
		if _, err := mapPeriod(result.TimePeriod); err != nil {
			return nil, fmt.Errorf("map tag result %d: %w", resultIndex, err)
		}
		for groupIndex, group := range result.Groups {
			if len(group.Keys) != 1 {
				return nil, fmt.Errorf("%w: tag group must have one key", ErrInvalidResponse)
			}
			metricValue, exists := group.Metrics[metric]
			if !exists {
				return nil, fmt.Errorf("%w: missing %s", ErrInvalidResponse, metric)
			}
			money, err := cost.ParseMoney(aws.ToString(metricValue.Amount), aws.ToString(metricValue.Unit))
			if err != nil {
				return nil, err
			}
			value := strings.TrimPrefix(group.Keys[0], tagKey+"$")
			if value == "" {
				value = "__untagged__"
			}
			values = append(values, tagcost.Cost{Provider: cost.ProviderCostExplorer, Basis: query.Basis, Window: query.Window, TagKey: tagKey, TagValue: value, Amount: money})
			_ = groupIndex
		}
	}
	if query.Window != cost.WindowMonthToDate {
		return values, nil
	}
	type key struct{ value, currency string }
	totals := map[key]tagcost.Cost{}
	for _, item := range values {
		k := key{item.TagValue, item.Amount.Currency()}
		existing, ok := totals[k]
		if !ok {
			totals[k] = item
			continue
		}
		sum, err := existing.Amount.Add(item.Amount)
		if err != nil {
			return nil, err
		}
		existing.Amount = sum
		totals[k] = existing
	}
	keys := make([]key, 0, len(totals))
	for k := range totals {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].value != keys[j].value {
			return keys[i].value < keys[j].value
		}
		return keys[i].currency < keys[j].currency
	})
	values = values[:0]
	for _, k := range keys {
		values = append(values, totals[k])
	}
	return values, nil
}
