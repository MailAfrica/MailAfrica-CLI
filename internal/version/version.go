// Package version holds the build version, overridable at link time:
//
//	go build -ldflags "-X github.com/MailAfrica/MailAfrica-CLI/internal/version.Version=v1.0.0"
package version

// Version is stamped at build time via -X. "dev" is the fallback for
// unreleased builds.
var Version = "dev"
