package presence

import (
	"errors"
	"sync"
	"time"
)

const lockTTL = 90 * time.Second

var ErrLocked = errors.New("estimate is locked by another user")

type EstimateLock struct {
	EstimateID string    `json:"estimateId"`
	UserID     string    `json:"userId"`
	UserName   string    `json:"userName"`
	CompanyID  string    `json:"companyId"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type LockConflict struct {
	Lock EstimateLock
}

func (e LockConflict) Error() string {
	return ErrLocked.Error()
}

func (e LockConflict) Unwrap() error {
	return ErrLocked
}

type EstimateLocks struct {
	mu    sync.RWMutex
	locks map[string]EstimateLock
}

func NewEstimateLocks() *EstimateLocks {
	return &EstimateLocks{
		locks: make(map[string]EstimateLock),
	}
}

func (l *EstimateLocks) Acquire(estimateID, userID, userName, companyID string) (EstimateLock, error) {
	now := time.Now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.purgeExpiredLocked(now)

	if existing, ok := l.locks[estimateID]; ok {
		if existing.UserID != userID {
			return EstimateLock{}, LockConflict{Lock: existing}
		}
		existing.UserName = userName
		existing.UpdatedAt = now
		l.locks[estimateID] = existing
		return existing, nil
	}

	lock := EstimateLock{
		EstimateID: estimateID,
		UserID:     userID,
		UserName:   userName,
		CompanyID:  companyID,
		UpdatedAt:  now,
	}
	l.locks[estimateID] = lock
	return lock, nil
}

func (l *EstimateLocks) Release(estimateID, userID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	existing, ok := l.locks[estimateID]
	if !ok || existing.UserID != userID {
		return
	}
	delete(l.locks, estimateID)
}

func (l *EstimateLocks) ReleaseAll(userID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for estimateID, lock := range l.locks {
		if lock.UserID == userID {
			delete(l.locks, estimateID)
		}
	}
}

func (l *EstimateLocks) ListByCompany(companyID string) []EstimateLock {
	return l.List(companyID, false)
}

func (l *EstimateLocks) List(companyID string, includeAll bool) []EstimateLock {
	now := time.Now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.purgeExpiredLocked(now)

	locks := make([]EstimateLock, 0)
	for _, lock := range l.locks {
		if includeAll || lock.CompanyID == companyID {
			locks = append(locks, lock)
		}
	}
	return locks
}

func (l *EstimateLocks) ForceRelease(estimateID string) (EstimateLock, bool) {
	now := time.Now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.purgeExpiredLocked(now)

	existing, ok := l.locks[estimateID]
	if !ok {
		return EstimateLock{}, false
	}

	delete(l.locks, estimateID)
	return existing, true
}

func (l *EstimateLocks) purgeExpiredLocked(now time.Time) {
	for estimateID, lock := range l.locks {
		if now.Sub(lock.UpdatedAt) > lockTTL {
			delete(l.locks, estimateID)
		}
	}
}
