package organizationmeta

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/organization"
)

type readerStub struct {
	values []organization.Account
	err    error
}

func (value readerStub) ReadAccounts(context.Context) ([]organization.Account, error) {
	return value.values, value.err
}
func TestCollectorPublishesAndPropagatesFailure(t *testing.T) {
	subject, err := New("payer", readerStub{values: []organization.Account{{Target: "payer", AccountID: "111111111111"}}})
	if err != nil {
		t.Fatal(err)
	}
	if subject.ID().Name != Name {
		t.Fatal("wrong ID")
	}
	result, err := subject.Collect(context.Background(), time.Now())
	if err != nil || len(result.Accounts()) != 1 {
		t.Fatalf("collect=%#v,%v", result, err)
	}
	expected := errors.New("failed")
	subject.reader = readerStub{err: expected}
	if _, err := subject.Collect(context.Background(), time.Now()); !errors.Is(err, expected) {
		t.Fatalf("error=%v", err)
	}
}
func TestNewRejectsInvalid(t *testing.T) {
	if subject, err := New("", readerStub{}); subject != nil || err == nil {
		t.Fatal("accepted empty target")
	}
	if subject, err := New("payer", nil); subject != nil || err == nil {
		t.Fatal("accepted nil reader")
	}
}
