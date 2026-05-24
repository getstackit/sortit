package auth

import "time"

type User struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	DisplayName string    `json:"displayName"`
	AvatarURL   string    `json:"avatarUrl,omitempty"`
	Email       string    `json:"email,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Principal struct {
	UserID      string `json:"userId"`
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	Email       string `json:"email,omitempty"`
}

func (p Principal) ActorName() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	if p.Login != "" {
		return p.Login
	}
	return "You"
}

func (p Principal) User() User {
	return User{
		ID:          p.UserID,
		Login:       p.Login,
		DisplayName: p.DisplayName,
		AvatarURL:   p.AvatarURL,
		Email:       p.Email,
	}
}

type OAuthUser struct {
	Provider       string
	ProviderUserID string
	Login          string
	DisplayName    string
	AvatarURL      string
	Email          string
}

type APIToken struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"tokenPrefix"`
	CreatedAt   time.Time  `json:"createdAt"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
}
