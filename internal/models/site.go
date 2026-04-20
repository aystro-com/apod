package models

import "time"

type Site struct {
	ID        int64     `json:"id"`
	Domain    string    `json:"domain"`
	Driver    string    `json:"driver"`
	Status    string    `json:"status"`
	RAM       string    `json:"ram"`
	CPU       string    `json:"cpu"`
	Storage   string    `json:"storage,omitempty"`
	Env       string    `json:"env"`
	Repo      string    `json:"repo,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Owner     string    `json:"owner,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
