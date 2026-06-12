package engine

import (
	"context"
	"fmt"

	"github.com/aystro/apod/internal/models"
)

// NeedsSetup reports whether the instance has no users yet (first-run state).
func (e *Engine) NeedsSetup() (bool, error) {
	n, err := e.db.CountUsers()
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// SetUpFirstAdmin creates the initial admin user with a login password, but
// only while the instance has no users. This is the web equivalent of the
// first CLI `apod user create --role admin`.
func (e *Engine) SetUpFirstAdmin(ctx context.Context, name, password string) (*models.User, error) {
	needs, err := e.NeedsSetup()
	if err != nil {
		return nil, err
	}
	if !needs {
		return nil, fmt.Errorf("setup already completed: an admin already exists")
	}
	if len(password) < minPasswordLength {
		return nil, fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}

	user, _, err := e.CreateUser(ctx, name, "admin")
	if err != nil {
		return nil, err
	}
	if err := e.SetUserPassword(name, password); err != nil {
		return nil, err
	}
	return user, nil
}
