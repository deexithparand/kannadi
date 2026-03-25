package azurerm

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

// LiveResource holds the fields we care about from the Azure API response
// for an azurerm_storage_account. These are the fields we compare against
// the state baseline to detect drift.
type LiveResource struct {
	Id                     string // ARM resource ID — primary match key against state
	Name                   string
	Location               string
	AccountKind            string // StorageV2, BlobStorage, etc.
	AccountTier            string // Standard | Premium  (split from SKU name)
	AccountReplicationType string // LRS, GRS, ZRS, etc. (split from SKU name)
}

// ListStorageAccounts queries Azure for all storage accounts in the subscription
// and returns them as LiveResource slices, ready for drift comparison.
//
// Auth is resolved via DefaultAzureCredential (env vars, managed identity, az cli, etc.)
// Subscription is read from AZURE_SUBSCRIPTION_ID env var.
func ListStorageAccounts() ([]LiveResource, error) {
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	if subscriptionID == "" {
		return nil, fmt.Errorf("AZURE_SUBSCRIPTION_ID env var not set")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure credential: %w", err)
	}

	client, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage accounts client: %w", err)
	}

	pager := client.NewListPager(nil)
	var results []LiveResource

	for pager.More() {
		page, err := pager.NextPage(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to list storage accounts: %w", err)
		}

		for _, account := range page.Value {
			// SKU name is "Standard_LRS", "Premium_ZRS" etc — split into tier + replication type
			tier, replicationType := parseSKU(account.SKU)

			results = append(results, LiveResource{
				Id:                     deref(account.ID),
				Name:                   deref(account.Name),
				Location:               deref(account.Location),
				AccountKind:            kindStr(account.Kind),
				AccountTier:            tier,
				AccountReplicationType: replicationType,
			})
		}
	}

	return results, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseSKU(sku *armstorage.SKU) (tier, replicationType string) {
	if sku == nil {
		return "", ""
	}
	parts := strings.SplitN(string(*sku.Name), "_", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return string(*sku.Name), ""
}

func kindStr(k *armstorage.Kind) string {
	if k == nil {
		return ""
	}
	return string(*k)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
