package domain

import (
	"fmt"
	"strings"
	"time"
)

type Tenant struct {
	ID        TenantID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type User struct {
	ID        UserID
	Email     string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TenantCreate struct {
	ID   TenantID
	Name string
	Now  time.Time
}

type UserCreate struct {
	ID    UserID
	Email string
	Name  string
	Now   time.Time
}

func NewTenant(input TenantCreate) (Tenant, error) {
	if emptyID(string(input.ID)) || strings.TrimSpace(input.Name) == "" {
		return Tenant{}, fmt.Errorf("tenant: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return Tenant{
		ID:        input.ID,
		Name:      strings.TrimSpace(input.Name),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func NewUser(input UserCreate) (User, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if emptyID(string(input.ID)) || email == "" || !strings.Contains(email, "@") {
		return User{}, fmt.Errorf("user: %w", ErrInvalidEntity)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return User{
		ID:        input.ID,
		Email:     email,
		Name:      strings.TrimSpace(input.Name),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
