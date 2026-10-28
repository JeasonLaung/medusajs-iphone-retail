package cart

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/sessionstore"
)

func TestCreateCommandBoundary(t *testing.T) {
	valid := CreateCommand{OperationID: "f8230181-a32c-4127-838b-92b7c033ef01", ScopeHash: sha256.Sum256([]byte("artificial scope")), StoreID: "store_one", RegionID: "region_one", Locale: "zh-CN"}
	if !validCreate(valid) {
		t.Fatal("valid internal creation rejected")
	}
	for _, edit := range []func(*CreateCommand){
		func(c *CreateCommand) { c.ScopeHash = [32]byte{} },
		func(c *CreateCommand) { c.OperationID = "F8230181-A32C-4127-838B-92B7C033EF01" },
		func(c *CreateCommand) { c.OperationID = "f8230181-a32c-1127-838b-92b7c033ef01" },
		func(c *CreateCommand) { c.StoreID = "store/id" },
		func(c *CreateCommand) { c.RegionID = "" },
		func(c *CreateCommand) { c.RegionID = strings.Repeat("a", 192) },
		func(c *CreateCommand) { c.Locale = "en\n" },
		func(c *CreateCommand) { c.Locale = "en_US" },
		func(c *CreateCommand) { c.Locale = strings.Repeat("en-", 20) },
	} {
		bad := valid
		edit(&bad)
		if validCreate(bad) {
			t.Fatal("invalid internal create accepted")
		}
	}
	ref := sessionstore.CartReference{CartID: "cart_one", StoreID: valid.StoreID, Generation: strings.Repeat("a", 64)}
	for _, version := range []string{"", "0", "01", "+1", "1.0", "1e1", " 1", "18446744073709551616"} {
		if r, err := LockForWrite(t.Context(), nil, valid.StoreID, ref, version); err != ErrInput || r != (Snapshot{}) {
			t.Fatal("invalid optimistic version reached storage")
		}
	}
}
