package main

import (
	"fmt"

	"github.com/deexithparand/kannadi/iac"
)

func main() {
	stateFiles := []string{
		"./iac/test/aws/aws_valid.tfstate",
		"./iac/test/azure/azurerm_valid.tfstate",
		"./iac/test/github/github_valid.tfstate",
	}

	for _, path := range stateFiles {
		fmt.Printf("\n── %s ──\n", path)
		resources, err := iac.StateReader(path)
		if err != nil {
			fmt.Printf("  error: %v\n", err)
			continue
		}
		for _, r := range resources {
			fmt.Printf("  [%s] %s  id=%s  provider=%s\n", r.Mode, r.Address, r.Id, r.Provider)
		}
	}
}
