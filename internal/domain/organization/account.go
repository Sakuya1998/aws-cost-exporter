// Package organization defines bounded AWS Organizations metadata.
package organization

import (
	"sort"

	"github.com/sakuya1998/aws-cost-exporter/internal/domain/identity"
)

// Account is non-sensitive metadata for one linked AWS account.
type Account struct {
	Target    identity.TargetID
	AccountID string
	Name      string
	Status    string
}

// Sort orders account metadata deterministically.
func Sort(values []Account) {
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].Target != values[right].Target {
			return values[left].Target < values[right].Target
		}
		return values[left].AccountID < values[right].AccountID
	})
}
