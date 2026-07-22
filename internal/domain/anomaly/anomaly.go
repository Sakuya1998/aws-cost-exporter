// Package anomaly contains bounded Cost Anomaly Detection summaries.
package anomaly

import (
	"sort"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/cost"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

type Summary struct {
	Target       identity.TargetID
	Active       bool
	Count        int
	Impact       cost.Money
	MaxImpact    cost.Money
	LastDetected time.Time
	HasImpact    bool
	HasMaxImpact bool
}

func Sort(values []Summary) {
	sort.SliceStable(values, func(i, j int) bool { return values[i].Target < values[j].Target })
}
