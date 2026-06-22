package presence

import (
	"errors"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	locks := NewEstimateLocks()

	lock, err := locks.Acquire("est-1", "user-1", "Иванов И.И.", "company-1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if lock.UserName != "Иванов И.И." {
		t.Fatalf("unexpected user name: %q", lock.UserName)
	}

	_, err = locks.Acquire("est-1", "user-2", "Петров П.П.", "company-1")
	var conflict LockConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("expected lock conflict, got %v", err)
	}
	if conflict.Lock.UserName != "Иванов И.И." {
		t.Fatalf("unexpected conflict user: %q", conflict.Lock.UserName)
	}

	locks.Release("est-1", "user-1")
	_, err = locks.Acquire("est-1", "user-2", "Петров П.П.", "company-1")
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
}

func TestListByCompanyAndTTL(t *testing.T) {
	locks := NewEstimateLocks()
	locks.mu.Lock()
	locks.locks["est-old"] = EstimateLock{
		EstimateID: "est-old",
		UserID:     "user-1",
		UserName:   "Иванов И.И.",
		CompanyID:  "company-1",
		UpdatedAt:  time.Now().UTC().Add(-2 * lockTTL),
	}
	locks.mu.Unlock()

	items := locks.ListByCompany("company-1")
	if len(items) != 0 {
		t.Fatalf("expected expired lock to be purged, got %d", len(items))
	}
}

func TestForceReleaseAllowsReacquire(t *testing.T) {
	locks := NewEstimateLocks()

	_, err := locks.Acquire("est-1", "user-1", "Иванов И.И.", "company-1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	released, ok := locks.ForceRelease("est-1")
	if !ok {
		t.Fatal("expected force release to succeed")
	}
	if released.UserID != "user-1" {
		t.Fatalf("unexpected released user: %q", released.UserID)
	}

	_, err = locks.Acquire("est-1", "user-1", "Иванов И.И.", "company-1")
	if err != nil {
		t.Fatalf("same user should re-acquire after force release: %v", err)
	}

	locks.ForceRelease("est-1")

	_, err = locks.Acquire("est-1", "user-2", "Петров П.П.", "company-1")
	if err != nil {
		t.Fatalf("other user should acquire after force release: %v", err)
	}
}

func TestListIncludeAll(t *testing.T) {
	locks := NewEstimateLocks()

	_, err := locks.Acquire("est-1", "user-1", "Иванов И.И.", "company-1")
	if err != nil {
		t.Fatalf("acquire company-1: %v", err)
	}
	_, err = locks.Acquire("est-2", "user-2", "Петров П.П.", "company-2")
	if err != nil {
		t.Fatalf("acquire company-2: %v", err)
	}

	if len(locks.List("company-1", false)) != 1 {
		t.Fatalf("expected one lock for company-1")
	}
	if len(locks.List("company-1", true)) != 2 {
		t.Fatalf("expected all locks for super admin view")
	}
}
