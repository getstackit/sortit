package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrCLILoginNotFound = errors.New("cli login not found")
var ErrCLILoginExpired = errors.New("cli login expired")
var ErrCLILoginPending = errors.New("cli login pending")
var ErrCLILoginConsumed = errors.New("cli login already consumed")

const (
	defaultCLILoginTTL     = 10 * time.Minute
	defaultCLIPollInterval = 1 * time.Second
)

type CLILoginStart struct {
	ID            string
	ExchangeToken string
	ExpiresAt     time.Time
	PollInterval  time.Duration
}

type CLILoginResult struct {
	User     User
	Metadata APIToken
	Token    string
}

type pendingCLILogin struct {
	exchangeTokenHash string
	expiresAt         time.Time
	result            *CLILoginResult
	consumed          bool
}

type cliLoginManager struct {
	mu     sync.Mutex
	logins map[string]*pendingCLILogin
}

func newCLILoginManager() *cliLoginManager {
	return &cliLoginManager{
		logins: make(map[string]*pendingCLILogin),
	}
}

func (m *cliLoginManager) Begin() (CLILoginStart, error) {
	id, _, err := newSecretToken("sortitcli")
	if err != nil {
		return CLILoginStart{}, err
	}
	exchangeToken, exchangeTokenHash, err := newSecretToken("sortitclix")
	if err != nil {
		return CLILoginStart{}, err
	}

	now := time.Now().UTC()
	start := CLILoginStart{
		ID:            id,
		ExchangeToken: exchangeToken,
		ExpiresAt:     now.Add(defaultCLILoginTTL),
		PollInterval:  defaultCLIPollInterval,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupLocked(now)
	m.logins[id] = &pendingCLILogin{
		exchangeTokenHash: exchangeTokenHash,
		expiresAt:         start.ExpiresAt,
	}

	return start, nil
}

func (m *cliLoginManager) MarkComplete(id string, result CLILoginResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	m.cleanupLocked(now)
	login, ok := m.logins[id]
	if !ok {
		return ErrCLILoginNotFound
	}
	if now.After(login.expiresAt) {
		delete(m.logins, id)
		return ErrCLILoginExpired
	}
	login.result = &result
	return nil
}

func (m *cliLoginManager) Exchange(id, exchangeToken string) (CLILoginResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	m.cleanupLocked(now)
	login, ok := m.logins[id]
	if !ok {
		return CLILoginResult{}, ErrCLILoginNotFound
	}
	if now.After(login.expiresAt) {
		delete(m.logins, id)
		return CLILoginResult{}, ErrCLILoginExpired
	}
	if hashToken(exchangeToken) != login.exchangeTokenHash {
		return CLILoginResult{}, ErrUnauthorized
	}
	if login.result == nil {
		return CLILoginResult{}, ErrCLILoginPending
	}
	if login.consumed {
		return CLILoginResult{}, ErrCLILoginConsumed
	}

	login.consumed = true
	result := *login.result
	delete(m.logins, id)
	return result, nil
}

func (m *cliLoginManager) cleanupLocked(now time.Time) {
	for id, login := range m.logins {
		if now.After(login.expiresAt) {
			delete(m.logins, id)
		}
	}
}

func (s *Service) BeginCLILogin() (CLILoginStart, error) {
	return s.cliLogins.Begin()
}

func (s *Service) CompleteCLILogin(ctx context.Context, id string, user User) (CLILoginResult, error) {
	metadata, rawToken, err := s.store.CreateAPIToken(ctx, user.ID, "CLI login")
	if err != nil {
		return CLILoginResult{}, err
	}

	result := CLILoginResult{
		User:     user,
		Metadata: metadata,
		Token:    rawToken,
	}
	if err := s.cliLogins.MarkComplete(id, result); err != nil {
		return CLILoginResult{}, err
	}
	return result, nil
}

func (s *Service) ExchangeCLILogin(id, exchangeToken string) (CLILoginResult, error) {
	return s.cliLogins.Exchange(id, exchangeToken)
}
