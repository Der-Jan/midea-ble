// Package mobile is the gomobile-facing facade for the internal/ac package.
//
// It deliberately exposes only primitive values, byte slices, errors, and small
// callback interfaces. Android implements Transport/Dialer and injects them here;
// the protocol and business logic remain in internal/ac.
package mobile
