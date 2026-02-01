package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.4"
	FeatureSet     = "Garbage Loop Fix + Strict 9999"
)

// PrintVersion prints the backend version to the console
func PrintVersion() {
	fmt.Printf("Backend Version: %s - %s\n", BackendVersion, FeatureSet)
}