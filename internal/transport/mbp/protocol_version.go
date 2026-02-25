package mbp

// CurrentProtocolVersion is the MBP protocol version spoken by this binary.
// Increment when adding features that require new message types or payload changes.
//
// Version history:
//
//	0 = legacy (pre-versioned binaries)
//	1 = initial versioned release
const CurrentProtocolVersion uint16 = 1

// MinSupportedProtocolVersion is the oldest protocol version this binary will
// accept from connecting peers. Peers below this version are hard-rejected.
// Set to 0 to accept all legacy (pre-versioned) peers.
//
// When raising this value, set DeprecatedProtocolVersion first for a full
// release-cycle warning window before enforcing the hard rejection.
var MinSupportedProtocolVersion uint16 = 0

// DeprecatedProtocolVersion is the lower bound of the deprecation window.
// Peers with version in [DeprecatedProtocolVersion, MinSupportedProtocolVersion)
// are accepted but log a WARN so operators know to upgrade before the next
// MinSupportedProtocolVersion bump. Set equal to MinSupportedProtocolVersion
// when no deprecation window is active.
var DeprecatedProtocolVersion uint16 = 0
