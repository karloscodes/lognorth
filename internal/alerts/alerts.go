// Package alerts decides when the owner hears about an app, and tells them.
// The detectors live here: new errors and regressions (errors.go), spikes,
// drift, and silence of an endpoint (spc_*.go and endpoints.go), an app that
// is down (uptime.go), and a leaked secret (leaks.go). So do the daily digest
// (digest.go), quiet hours, and the channels that deliver (deliver.go,
// smtp.go).
package alerts
