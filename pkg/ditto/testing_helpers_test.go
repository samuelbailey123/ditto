// Shared test types used by both the white-box (package ditto) and black-box
// (package ditto_test) test files. This file belongs to package ditto so that
// unexported symbols defined here are accessible to helpers_internal_test.go.
package ditto

// fatalSignal is the panic value emitted by stub testing.TB implementations
// when Fatal or Fatalf is called. Callers that want to verify the call must
// recover this value and check its type.
type fatalSignal struct{ msg string }
