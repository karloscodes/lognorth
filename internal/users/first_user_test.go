package users

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/karloscodes/cartridge/sqlite"
)

func TestCreateFirstUser(t *testing.T) {
	t.Run("with many setup requests at once, only one owner is created", func(t *testing.T) {
		manager := sqlite.NewManager(sqlite.Config{Path: filepath.Join(t.TempDir(), "users.db"), MaxOpenConns: 4, MaxIdleConns: 4})
		t.Cleanup(func() { _ = manager.Close() })
		db, err := manager.Connect()
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		if err := db.AutoMigrate(&User{}); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		const requests = 8
		errs := make([]error, requests)
		start := make(chan struct{})
		var wg sync.WaitGroup

		for i := range requests {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errs[i] = CreateFirstUser(manager, fmt.Sprintf("owner%d@example.com", i), "correct-horse")
			}()
		}
		close(start)
		wg.Wait()

		var count int64
		db.Model(&User{}).Count(&count)
		if count != 1 {
			t.Errorf("users = %d, want 1", count)
		}
		created := 0
		for _, err := range errs {
			switch {
			case err == nil:
				created++
			case !errors.Is(err, ErrSetupDone):
				t.Errorf("err = %v, want nil or ErrSetupDone", err)
			}
		}
		if created != 1 {
			t.Errorf("created = %d, want 1", created)
		}
	})
}
