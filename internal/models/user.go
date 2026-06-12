package models

import "time"

type User struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	UID         int       `json:"uid"`
	Role        string    `json:"role"`
	APIKey      string    `json:"api_key,omitempty"`
	HasPassword bool      `json:"has_password"`
	CreatedAt   time.Time `json:"created_at"`
}
