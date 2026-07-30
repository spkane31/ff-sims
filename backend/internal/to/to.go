// Package to provides small generic conversion helpers.
package to

// Ptr returns a pointer to value.
func Ptr[T any](value T) *T { return &value }
