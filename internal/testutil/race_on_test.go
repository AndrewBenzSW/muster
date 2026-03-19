//go:build race

package testutil

// raceEnabled is set to true when the race detector is enabled via -race flag.
func init() {
	raceEnabled = true
}
