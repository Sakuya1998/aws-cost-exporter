package cost

// Provider identifies the billing backend that produced a cost value.
type Provider string

// CostProvider is the public domain name used by the v0.3 contract.
type CostProvider = Provider

const (
	ProviderCostExplorer Provider = "cost_explorer"
	ProviderCURAthena    Provider = "cur_athena"
)

// Valid reports whether the provider is one of the bounded public values.
func (value Provider) Valid() bool {
	return value == ProviderCostExplorer || value == ProviderCURAthena
}

// Basis identifies the accounting basis used for a cost value.
type Basis string

// CostBasis is the public domain name used by the v0.3 contract.
type CostBasis = Basis

const (
	BasisUnblended Basis = "unblended"
	BasisAmortized Basis = "amortized"
	BasisNet       Basis = "net"
)

// Valid reports whether the basis is one of the supported values.
func (value Basis) Valid() bool {
	return value == BasisUnblended || value == BasisAmortized || value == BasisNet
}
