package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.1"
	FeatureSet     = "Chunked Upload Support + Filename Fix"
)

// PrintVersion prints the backend version to the console
func PrintVersion() {
	fmt.Printf("Backend Version: %s - %s\n", BackendVersion, FeatureSet)
}