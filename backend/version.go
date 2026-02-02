package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.6.0"
	FeatureSet     = "Split Mercadorias Report by CFOP Type"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}
