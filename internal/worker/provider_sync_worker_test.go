package worker

import "testing"

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
