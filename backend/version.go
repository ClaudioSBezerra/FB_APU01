package main

import "fmt"

// Version information for backend deployment validation
const (
	BackendVersion = "4.6.2"
	FeatureSet     = "Fix 403 on Reset DB (Promote Admin)"
)

func GetVersionInfo() string {
	return fmt.Sprintf("Backend Version: %s | Features: %s", BackendVersion, FeatureSet)
}

func PrintVersion() {
	fmt.Println(GetVersionInfo())
}
