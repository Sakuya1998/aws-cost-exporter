// Package identity defines stable target and collector identities.
package identity

import "strings"

// TargetID is the bounded public name of one configured AWS target.
type TargetID string

// CollectorID uniquely identifies one collector inside one target.
type CollectorID struct {
	Target TargetID
	Name   string
}

// Valid reports whether both identity components are non-empty and normalized.
func (id CollectorID) Valid() bool {
	return strings.TrimSpace(string(id.Target)) == string(id.Target) && id.Target != "" &&
		strings.TrimSpace(id.Name) == id.Name && id.Name != ""
}

// String returns the stable log/debug representation. Metrics use separate labels.
func (id CollectorID) String() string { return string(id.Target) + "/" + id.Name }
