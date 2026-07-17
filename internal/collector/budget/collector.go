// Package budget collects target allowlisted AWS Budgets.
package budget

import (
	"context"
	"errors"
	"time"

	domain "github.com/sakuya1998/aws-cost-exporter/internal/domain/budget"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

const Name = "budgets"

type Reader interface {
	ReadBudgets(context.Context) ([]domain.Budget, error)
}

type Collector struct {
	id     identity.CollectorID
	reader Reader
}

func New(target identity.TargetID, reader Reader) (*Collector, error) {
	if target == "" || reader == nil {
		return nil, errors.New("invalid Budgets collector configuration")
	}
	return &Collector{id: identity.CollectorID{Target: target, Name: Name}, reader: reader}, nil
}

func (collector *Collector) ID() identity.CollectorID { return collector.id }
func (collector *Collector) Collect(ctx context.Context, _ time.Time) (snapshot.PartialSnapshot, error) {
	values, err := collector.reader.ReadBudgets(ctx)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	return snapshot.New(nil, nil, values, nil), nil
}
