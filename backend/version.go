package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.8.0"
	FeatureSet     = "Migration: CompanyID for Data Ownership (Removed CNPJ Dependency)"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}

func PrintVersion() {
	fmt.Println(GetVersionInfo())
}
