// Package ports defines infrastructure capabilities required by application code.
package ports

import "time"

// Clock supplies time without coupling application logic to the system clock.
type Clock interface {
	Now() time.Time
}
