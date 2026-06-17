package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nav-saas-mvp/backend/internal/domain"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrNotAuthorized      = errors.New("user is not authorized")
	ErrInvalidToken       = errors.New("invalid token")
	ErrTokenExpired       = errors.New("token expired")
)

type UserReader interface {
	FindUserByCompanyAndName(companyName, userName string) (domain.User, bool)
}

type Service struct {
	users     UserReader
	jwtSecret []byte
}

func NewService(users UserReader, jwtSecret string) *Service {
	return &Service{
		users:     users,
		jwtSecret: []byte(jwtSecret),
	}
}

func NewPassword(password string) (hash string, salt string, err error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}

	salt = hex.EncodeToString(saltBytes)
	return HashPassword(password, salt), salt, nil
}

func HashPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func VerifyPassword(password, salt, expectedHash string) bool {
	actual := HashPassword(password, salt)
	return hmac.Equal([]byte(actual), []byte(expectedHash))
}

func (s *Service) Login(companyName, userName, password string) (string, domain.User, error) {
	user, ok := s.users.FindUserByCompanyAndName(
		strings.TrimSpace(companyName),
		strings.TrimSpace(userName),
	)
	if !ok || !VerifyPassword(password, user.PasswordSalt, user.PasswordHash) {
		return "", domain.User{}, ErrInvalidCredentials
	}

	if !user.CanAccessApp() && !user.CanAccessAdmin() {
		return "", domain.User{}, ErrNotAuthorized
	}

	token, err := s.Sign(domain.Claims{
		UserID:    user.ID,
		CompanyID: user.CompanyID,
		Email:     user.Email,
		Role:      user.Role(),
		ExpiresAt: time.Now().Add(12 * time.Hour).Unix(),
	})
	if err != nil {
		return "", domain.User{}, err
	}

	return token, user, nil
}

func (s *Service) Sign(claims domain.Claims) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerPart := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsPart := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := headerPart + "." + claimsPart
	signature := s.signature(unsigned)

	return unsigned + "." + signature, nil
}

func (s *Service) Verify(token string) (domain.Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domain.Claims{}, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(s.signature(unsigned)), []byte(parts[2])) {
		return domain.Claims{}, ErrInvalidToken
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.Claims{}, ErrInvalidToken
	}

	var claims domain.Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return domain.Claims{}, ErrInvalidToken
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return domain.Claims{}, ErrTokenExpired
	}

	return claims, nil
}

func (s *Service) signature(unsigned string) string {
	mac := hmac.New(sha256.New, s.jwtSecret)
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func BearerToken(header string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", fmt.Errorf("%w: missing bearer prefix", ErrInvalidToken)
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", ErrInvalidToken
	}

	return token, nil
}
