package presence

import (
	"sync"
	"time"
)

const licenseSessionTTL = 90 * time.Second

type LicenseSession struct {
	SubsectionID string    `json:"subsectionId"`
	UserID       string    `json:"userId"`
	UserName     string    `json:"userName"`
	CompanyID    string    `json:"companyId"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type LicenseBlocked struct {
	SubsectionID string `json:"subsectionId"`
	Name         string `json:"name"`
}

type LicenseSyncResult struct {
	Granted bool             `json:"granted"`
	Held    []string         `json:"held,omitempty"`
	Blocked []LicenseBlocked `json:"blocked,omitempty"`
}

type LicenseSessions struct {
	mu       sync.RWMutex
	sessions map[string]LicenseSession
}

func NewLicenseSessions() *LicenseSessions {
	return &LicenseSessions{
		sessions: make(map[string]LicenseSession),
	}
}

func licenseSessionKey(companyID, subsectionID, userID string) string {
	return companyID + "|" + subsectionID + "|" + userID
}

func (l *LicenseSessions) Sync(
	companyID, userID, userName string,
	subsectionIDs []string,
	availableFor func(subsectionID string) int,
	nameFor func(subsectionID string) string,
) LicenseSyncResult {
	now := time.Now().UTC()
	wanted := make(map[string]struct{}, len(subsectionIDs))
	ordered := make([]string, 0, len(subsectionIDs))
	for _, id := range subsectionIDs {
		if id == "" {
			continue
		}
		if _, ok := wanted[id]; ok {
			continue
		}
		wanted[id] = struct{}{}
		ordered = append(ordered, id)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.purgeExpiredLocked(now)

	blocked := make([]LicenseBlocked, 0)
	for _, subsectionID := range ordered {
		key := licenseSessionKey(companyID, subsectionID, userID)
		if _, ok := l.sessions[key]; ok {
			continue
		}
		if l.countActiveLocked(companyID, subsectionID) >= availableFor(subsectionID) {
			blocked = append(blocked, LicenseBlocked{
				SubsectionID: subsectionID,
				Name:         nameFor(subsectionID),
			})
		}
	}
	if len(blocked) > 0 {
		return LicenseSyncResult{Granted: false, Blocked: blocked}
	}

	for key, session := range l.sessions {
		if session.UserID != userID || session.CompanyID != companyID {
			continue
		}
		if _, ok := wanted[session.SubsectionID]; !ok {
			delete(l.sessions, key)
		}
	}

	held := make([]string, 0, len(ordered))
	for _, subsectionID := range ordered {
		key := licenseSessionKey(companyID, subsectionID, userID)
		session := LicenseSession{
			SubsectionID: subsectionID,
			UserID:       userID,
			UserName:     userName,
			CompanyID:    companyID,
			UpdatedAt:    now,
		}
		l.sessions[key] = session
		held = append(held, subsectionID)
	}

	return LicenseSyncResult{Granted: true, Held: held}
}

func (l *LicenseSessions) ReleaseAll(userID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, session := range l.sessions {
		if session.UserID == userID {
			delete(l.sessions, key)
		}
	}
}

func (l *LicenseSessions) countActiveLocked(companyID, subsectionID string) int {
	count := 0
	for _, session := range l.sessions {
		if session.CompanyID == companyID && session.SubsectionID == subsectionID {
			count++
		}
	}
	return count
}

func (l *LicenseSessions) purgeExpiredLocked(now time.Time) {
	for key, session := range l.sessions {
		if now.Sub(session.UpdatedAt) > licenseSessionTTL {
			delete(l.sessions, key)
		}
	}
}
