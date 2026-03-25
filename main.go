package main

import (
	"fmt"

	"github.com/deexithparand/kannadi/enumeration/azurerm"
	"github.com/deexithparand/kannadi/iac"
)

func main() {
	// ── 1. read state baseline ────────────────────────────────────────────────
	stateResources, err := iac.StateReader("./iac/test/azure/azurerm_valid.tfstate")
	if err != nil {
		fmt.Println("state error:", err)
		return
	}

	fmt.Println("── state baseline ──")
	for _, r := range stateResources {
		if r.Type == "azurerm_storage_account" {
			fmt.Printf("  %s  id=%s\n", r.Address, r.Id)
		}
	}

	// ── 2. enumerate live resources from Azure ────────────────────────────────
	fmt.Println("\n── live (Azure API) ──")
	liveResources, err := azurerm.ListStorageAccounts()
	if err != nil {
		fmt.Println("enumeration error:", err)
		return
	}

	for _, r := range liveResources {
		fmt.Printf("  %s  name=%s  location=%s  kind=%s  tier=%s  replication=%s\n",
			r.Id, r.Name, r.Location, r.AccountKind, r.AccountTier, r.AccountReplicationType)
	}
}
