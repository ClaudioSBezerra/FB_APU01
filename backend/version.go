package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.6.1"
	FeatureSet     = "CFOP Fallback to Type O & Aggregation Filter Update"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}

func PrintVersion() {
	fmt.Println(GetVersionInfo())
}
