package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.3"
	FeatureSet     = "LPAD Fix + Line Trimming"
)

// PrintVersion prints the backend version to the console
func PrintVersion() {
	fmt.Printf("Backend Version: %s - %s\n", BackendVersion, FeatureSet)
}