// Package tarotest holds the contract test suites every [tarodomain.Provider]
// must pass.
//
// A suite is a function such as [EventStoreContract] that takes a [T] and a
// [Factory]. T is a small interface instead of *testing.T because the
// Cloudflare harness runs the suites inside workerd, where there is no
// "go test". Wrap adapts a *testing.T.
//
// A provider's test file hands over a factory that returns a provider with
// empty storage; each case asks it for a fresh one:
//
//	func TestEventStoreContract(t *testing.T) {
//		tarotest.EventStoreContract(tarotest.Wrap(t), newProvider)
//	}
//
// The suites never depend on wall-clock time: they compare with timestamps the
// store returned, or with cut-offs far in the past or the future.
package tarotest
