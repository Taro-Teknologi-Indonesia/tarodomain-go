// Package tarodomain is the Go implementation of the TaroDomain framework
// specification: event sourcing, CQRS and DDD with strong or eventual
// consistency per command.
//
// The core never depends on a database driver, an HTTP framework or a cloud
// SDK. Providers live in their own packages and implement the persistence and
// event bus contracts defined here.
package tarodomain
