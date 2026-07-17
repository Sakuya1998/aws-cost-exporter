// Package organizationmeta collects target Organizations account metadata.
package organizationmeta

import (
	"context"
	"errors"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
	"github.com/sakuya1998/aws-cost-exporter/internal/domain/snapshot"
)

const Name = "organizations"

type Reader interface {
	ReadAccounts(context.Context) ([]organization.Account, error)
}

type Collector struct {
	id     identity.CollectorID
	reader Reader
}

func New(target identity.TargetID, reader Reader) (*Collector, error) {
	if target == "" || reader == nil {
		return nil, errors.New("invalid Organizations collector configuration")
	}
	return &Collector{id: identity.CollectorID{Target: target, Name: Name}, reader: reader}, nil
}

func (collector *Collector) ID() identity.CollectorID { return collector.id }
func (collector *Collector) Collect(ctx context.Context, _ time.Time) (snapshot.PartialSnapshot, error) {
	values, err := collector.reader.ReadAccounts(ctx)
	if err != nil {
		return snapshot.PartialSnapshot{}, err
	}
	return snapshot.New(nil, nil, nil, values), nil
}
