package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.5.0"
	FeatureSet     = "Reset Database Admin Tool"
)

// PrintVersion prints the backend version to the console
func PrintVersion() {
	fmt.Printf("Backend Version: %s - %s\n", BackendVersion, FeatureSet)
}
