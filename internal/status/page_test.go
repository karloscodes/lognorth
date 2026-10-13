package status

import (
	"errors"
	"testing"

	"lognorth/internal/apikeys"
)

func TestCreatePage(t *testing.T) {
	t.Run("makes a slug from the name, and a free one for a second page of that name", func(t *testing.T) {
		db := newTestDB(t)

		first, err1 := CreatePage(db, "Fusionaly Pro", "", nil)
		second, err2 := CreatePage(db, "Fusionaly Pro!", "", nil)

		if err1 != nil || err2 != nil || first.Slug != "fusionaly-pro" || second.Slug != "fusionaly-pro-2" {
			t.Errorf("expected fusionaly-pro and fusionaly-pro-2, got %q and %q (errors %v, %v)", first.Slug, second.Slug, err1, err2)
		}
	})

	t.Run("keeps the domain bare and lower case", func(t *testing.T) {
		db := newTestDB(t)

		page, err := CreatePage(db, "Shop", " https://Status.Shop.example/ ", nil)

		if err != nil || page.Domain != "status.shop.example" {
			t.Errorf("expected status.shop.example, got %q (error %v)", page.Domain, err)
		}
	})

	t.Run("refuses a domain that is not one, taken, or LogNorth's own", func(t *testing.T) {
		db := newTestDB(t)
		CreatePage(db, "Shop", "status.shop.example", nil)

		_, bad := CreatePage(db, "Blog", "not a domain", nil)
		_, taken := CreatePage(db, "Blog", "status.shop.example", nil)
		_, own := CreatePage(db, "Blog", "logs.example.com", []string{"logs.example.com:443"})
		_, ok := CreatePage(db, "Blog", "status.blog.example", []string{"logs.example.com"})

		if !errors.Is(bad, ErrBadDomain) || !errors.Is(taken, ErrDomainTaken) || !errors.Is(own, ErrOwnDomain) {
			t.Errorf("expected ErrBadDomain, ErrDomainTaken, ErrOwnDomain, got %v, %v, %v", bad, taken, own)
		}
		if ok != nil {
			t.Errorf("expected a free domain to save, got %v", ok)
		}
	})
}

func TestPageByHost(t *testing.T) {
	db := newTestDB(t)
	page := newPage(t, db, "Shop", "status.shop.example")

	found, ok := PageByHost(db, "Status.Shop.example:443")
	_, other := PageByHost(db, "logs.example.com")

	if !ok || found.ID != page.ID {
		t.Errorf("expected the Shop page for its host with a port, got %+v", found)
	}
	if other {
		t.Errorf("expected no page for another host")
	}
}

func TestDeletePage(t *testing.T) {
	db := newTestDB(t)
	page := newPage(t, db, "Shop", "")
	app := newApp(t, db, "shop", page.ID)

	err := DeletePage(db, page.ID)

	var fresh apikeys.APIKey
	db.First(&fresh, app.ID)
	_, still := PageBySlug(db, "shop")
	if err != nil || fresh.StatusPageID != 0 || still {
		t.Errorf("expected the page gone and its app on no page, got page id %d, page found %v, error %v", fresh.StatusPageID, still, err)
	}
}

func TestSetAppPage(t *testing.T) {
	db := newTestDB(t)
	page := newPage(t, db, "Shop", "")
	app := newApp(t, db, "shop", 0)

	moved := SetAppPage(db, app.ID, page.ID)
	missing := SetAppPage(db, app.ID, page.ID+1)

	var fresh apikeys.APIKey
	db.First(&fresh, app.ID)
	if moved != nil || fresh.StatusPageID != page.ID {
		t.Errorf("expected the app on the Shop page, got %d (error %v)", fresh.StatusPageID, moved)
	}
	if !errors.Is(missing, ErrPageNotFound) {
		t.Errorf("expected ErrPageNotFound for a page that does not exist, got %v", missing)
	}
}
