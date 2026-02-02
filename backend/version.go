package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.6.4"
	FeatureSet     = "Reset Admin Password (123456)"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}

func PrintVersion() {
	fmt.Println(GetVersionInfo())
}
