package status

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"lognorth/internal/apikeys"
)

var (
	ErrPageNameRequired = errors.New("Name can't be empty")
	ErrPageNameTooLong  = errors.New("Too long — 60 characters max")
	ErrBadDomain        = errors.New("That isn't a domain. Use one like status.example.com")
	ErrDomainTaken      = errors.New("Another status page has that domain")
	ErrOwnDomain        = errors.New("LogNorth answers on that domain. Use another one, like status.example.com")
	ErrPageNotFound     = errors.New("Can't find that status page")
)

// Page is one public status page: the apps of one product under one name.
// It answers at /status/<slug>, and at its own domain when it has one.
type Page struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:60;not null" json:"name"`
	Slug      string    `gorm:"size:60;not null;uniqueIndex" json:"slug"`
	Domain    string    `gorm:"size:253;index" json:"domain"` // "" when it has none
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Page) TableName() string { return "status_pages" }

var domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// Pages lists the status pages by name.
func Pages(db *gorm.DB) []Page {
	pages := []Page{}
	db.Order("name").Find(&pages)
	return pages
}

// PageBySlug finds the page at /status/<slug>.
func PageBySlug(db *gorm.DB, slug string) (Page, bool) {
	var page Page
	err := db.Where("slug = ?", slug).First(&page).Error
	return page, err == nil
}

// PageByHost finds the page whose domain is the host of a request. The host
// may carry a port.
func PageByHost(db *gorm.DB, host string) (Page, bool) {
	host = HostOnly(host)
	if host == "" {
		return Page{}, false
	}
	var page Page
	err := db.Where("domain = ?", host).First(&page).Error
	return page, err == nil
}

// CreatePage adds a page. ownHosts are the hosts LogNorth itself answers on:
// a page can't take one, or the dashboard would turn into the page.
func CreatePage(db *gorm.DB, name, domain string, ownHosts []string) (Page, error) {
	name, domain, err := checkPage(db, 0, name, domain, ownHosts)
	if err != nil {
		return Page{}, err
	}
	page := Page{Name: name, Slug: freeSlug(db, name), Domain: domain}
	return page, db.Create(&page).Error
}

// UpdatePage renames a page or changes its domain. The slug stays, so its
// /status/<slug> link keeps working.
func UpdatePage(db *gorm.DB, id uint, name, domain string, ownHosts []string) error {
	if err := db.First(&Page{}, id).Error; err != nil {
		return ErrPageNotFound
	}
	name, domain, err := checkPage(db, id, name, domain, ownHosts)
	if err != nil {
		return err
	}
	return db.Model(&Page{}).Where("id = ?", id).Updates(map[string]any{"name": name, "domain": domain}).Error
}

// DeletePage removes a page. Its apps stay, on no page.
func DeletePage(db *gorm.DB, id uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&apikeys.APIKey{}).Where("status_page_id = ?", id).Update("status_page_id", 0).Error; err != nil {
			return err
		}
		return tx.Delete(&Page{}, id).Error
	})
}

// SetAppPage puts an app on a page, or on no page with 0. An app is on one
// page at most.
func SetAppPage(db *gorm.DB, appID, pageID uint) error {
	if pageID != 0 {
		if err := db.First(&Page{}, pageID).Error; err != nil {
			return ErrPageNotFound
		}
	}
	return db.Model(&apikeys.APIKey{}).Where("id = ?", appID).Update("status_page_id", pageID).Error
}

// HostOnly is a host as a page stores it: lower case, with no port.
func HostOnly(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}

// checkPage trims and checks a page's name and domain.
func checkPage(db *gorm.DB, id uint, name, domain string, ownHosts []string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", ErrPageNameRequired
	}
	if utf8.RuneCountInString(name) > 60 {
		return "", "", ErrPageNameTooLong
	}

	// Take "https://status.example.com/" as well as the bare domain.
	domain = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "https://"), "http://")
	domain = strings.TrimSuffix(domain, "/")
	if domain == "" {
		return name, "", nil
	}
	if len(domain) > 253 || !domainPattern.MatchString(domain) {
		return "", "", ErrBadDomain
	}
	for _, own := range ownHosts {
		if HostOnly(own) == domain {
			return "", "", ErrOwnDomain
		}
	}
	var taken int64
	db.Model(&Page{}).Where("domain = ? AND id <> ?", domain, id).Count(&taken)
	if taken > 0 {
		return "", "", ErrDomainTaken
	}
	return name, domain, nil
}

// freeSlug makes a slug from the name that no other page has: "Fusionaly
// Pro" is fusionaly-pro, then fusionaly-pro-2.
func freeSlug(db *gorm.DB, name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	base := strings.TrimSuffix(b.String(), "-")
	if base == "" {
		base = "status"
	}

	slug := base
	for n := 2; ; n++ {
		var taken int64
		db.Model(&Page{}).Where("slug = ?", slug).Count(&taken)
		if taken == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, n)
	}
}
