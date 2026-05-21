// Package version defines the contract for how standalone probe modules declare their semver
// for static binary publishing. The probes core does not import any specific probe.
//
// Convention: add cmd/version.go in package main alongside cmd/main.go with exactly:
//
//	var Version = "x.y.z"
//
// Release tooling reads that assignment textually.
package version

// VersionGoFileRelativePath is relative to the probe module root (directory containing go.mod).
const VersionGoFileRelativePath = "cmd/version.go"

// VersionVarName is the variable identifier in package main within that file.
const VersionVarName = "Version"
