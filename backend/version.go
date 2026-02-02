package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.7.0"
	FeatureSet     = "Company Data Reset & Segregation"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}

func PrintVersion() {
	fmt.Println(GetVersionInfo())
}
