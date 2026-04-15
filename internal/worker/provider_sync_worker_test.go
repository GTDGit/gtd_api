package worker

import (
	"testing"

	"github.com/GTDGit/gtd_api/internal/models"
)

func TestSyncedAdminPreservesExistingValue(t *testing.T) {
	t.Parallel()

	admin := syncedAdmin(2500, nil)
	if admin == nil || *admin != 2500 {
		t.Fatalf("syncedAdmin(nil) = %#v, want 2500", admin)
	}

	updated := 3000
	admin = syncedAdmin(2500, &updated)
	if admin == nil || *admin != 3000 {
		t.Fatalf("syncedAdmin(updated) = %#v, want 3000", admin)
	}
}

func TestShouldPreserveProviderSKUAvailabilityForUATAlias(t *testing.T) {
	t.Parallel()

	if !shouldPreserveProviderSKUAvailability(models.PPOBProviderSKU{SkuCode: "9900446"}) {
		t.Fatalf("expected 99-prefixed SKU to be preserved")
	}
	if shouldPreserveProviderSKUAvailability(models.PPOBProviderSKU{SkuCode: "2201001"}) {
		t.Fatalf("expected normal SKU to follow sync availability")
	}
}
