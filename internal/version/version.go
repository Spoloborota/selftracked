// Package version carries the build identity of the selftracked binary.
package version

// Version is the semantic version of this build. It is overridden at release
// time with -ldflags; the default marks a build made from a working tree.
var Version = "0.0.0-dev"

// String returns the version as it should appear in output.
func String() string { return "selftracked " + Version }
