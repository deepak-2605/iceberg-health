// Command icehealth reports health metrics for Apache Iceberg tables:
// snapshot bloat, stale snapshots, schema evolution, and (live tables only)
// partition skew — via a CLI and a small web dashboard.
package main

import "os"

func main() {
	// Cobra prints the error itself; main only maps failure to an exit code.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
