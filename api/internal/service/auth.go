package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

// Interfaces declaradas aqui, só com o que o AuthService usa.
// *repository.Queries e *steam.Client as satisfazem sem declarar nada.
type UserRepository interface {
	UpsertUserBySteamID(ctx context.Context, arg repository.UpsertUserBySteamIDParams) (repository.User, error)
	GetUserByID(ctx context.Context, id int64) (repository.User, error)
	ListDemoUsers(ctx context.Context) ([]repository.User, error)
	SetTradeURL(ctx context.Context, arg repository.SetTradeURLParams) (repository.User, error)
	SetSteamAPIKey(ctx context.Context, arg repository.SetSteamAPIKeyParams) (repository.User, error)
}

type SteamAuth interface {
	LoginURL(state string) string
	VerifyOpenID(ctx context.Context, q url.Values) (string, error)
	GetPlayerSummary(ctx context.Context, steamID string) (steam.PlayerSummary, error)
	ValidateAPIKey(ctx context.Context, apiKey string) error
}

type AuthService struct {
	users UserRepository
	steam SteamAuth
	keys  KeyBox // nil: cadastro de API key desligado
}

func NewAuthService(users UserRepository, steam SteamAuth, keys KeyBox) *AuthService {
	return &AuthService{users: users, steam: steam, keys: keys}
}

func (s *AuthService) LoginURL(state string) string {
	return s.steam.LoginURL(state)
}

// LoginWithSteam valida o retorno da Steam, busca o perfil e cria/atualiza o usuário.
func (s *AuthService) LoginWithSteam(ctx context.Context, q url.Values) (domain.User, error) {
	steamID, err := s.steam.VerifyOpenID(ctx, q)
	if err != nil {
		if errors.Is(err, steam.ErrInvalidAssertion) {
			return domain.User{}, fmt.Errorf("%w: %v", domain.ErrUnauthenticated, err)
		}
		return domain.User{}, fmt.Errorf("verify steam login: %w", err)
	}

	profile, err := s.steam.GetPlayerSummary(ctx, steamID)
	if err != nil {
		return domain.User{}, fmt.Errorf("fetch steam profile: %w", err)
	}

	u, err := s.users.UpsertUserBySteamID(ctx, repository.UpsertUserBySteamIDParams{
		SteamID:     steamID,
		DisplayName: profile.PersonaName,
		AvatarURL:   profile.AvatarFull,
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("upsert user: %w", err)
	}
	return toDomainUser(u), nil
}

func (s *AuthService) GetUser(ctx context.Context, id int64) (domain.User, error) {
	u, err := s.users.GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return toDomainUser(u), nil
}

// DemoUsers lista as contas de demonstração (só usado com DEV_LOGIN ligado).
func (s *AuthService) DemoUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := s.users.ListDemoUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list demo users: %w", err)
	}
	out := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDomainUser(r))
	}
	return out, nil
}

// DemoLogin valida que o usuário é uma conta demo. Contas reais só entram pela Steam.
func (s *AuthService) DemoLogin(ctx context.Context, userID int64) (domain.User, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if !u.IsDemo() {
		return domain.User{}, domain.ErrForbidden
	}
	return u, nil
}

// SetTradeURL valida e salva a trade URL do usuário (forma canônica).
func (s *AuthService) SetTradeURL(ctx context.Context, userID int64, raw string) (domain.User, error) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	normalized, err := domain.NormalizeTradeURL(raw, u)
	if err != nil {
		return domain.User{}, err
	}
	row, err := s.users.SetTradeURL(ctx, repository.SetTradeURLParams{ID: userID, TradeURL: &normalized})
	if err != nil {
		return domain.User{}, fmt.Errorf("set trade url: %w", err)
	}
	return toDomainUser(row), nil
}

func toDomainUser(u repository.User) domain.User {
	return domain.User{
		ID:               u.ID,
		SteamID:          u.SteamID,
		DisplayName:      u.DisplayName,
		AvatarURL:        u.AvatarURL,
		BalanceCents:     u.BalanceCents,
		TradeURL:         u.TradeURL,
		HasSteamAPIKey:   len(u.SteamAPIKeyEnc) > 0,
		SteamAPIKeyLast4: u.SteamAPIKeyLast4,
		Role:             u.Role,
		DebtCents:        u.DebtCents,
		CreatedAt:        u.CreatedAt,
	}
}

// A Web API key da Steam tem 32 caracteres hexadecimais.
var apiKeyRe = regexp.MustCompile(`^[0-9A-F]{32}$`)

// SetSteamAPIKey valida a key na própria Steam e a guarda criptografada.
// Ela nunca volta em nenhuma resposta: só os 4 últimos caracteres.
func (s *AuthService) SetSteamAPIKey(ctx context.Context, userID int64, raw string) (domain.User, error) {
	if s.keys == nil {
		return domain.User{}, domain.ErrFeatureDisabled
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if u.IsDemo() {
		return domain.User{}, domain.ErrDemoAccount
	}

	key := strings.ToUpper(strings.TrimSpace(raw))
	if !apiKeyRe.MatchString(key) {
		return domain.User{}, fmt.Errorf("%w: steam api key must have 32 hexadecimal characters", domain.ErrInvalidInput)
	}
	if err := s.steam.ValidateAPIKey(ctx, key); err != nil {
		if errors.Is(err, steam.ErrInvalidAPIKey) {
			return domain.User{}, domain.ErrInvalidAPIKey
		}
		return domain.User{}, fmt.Errorf("validate api key: %w", err)
	}

	sealed, err := s.keys.Seal([]byte(key), apiKeyAAD(userID))
	if err != nil {
		return domain.User{}, fmt.Errorf("seal api key: %w", err)
	}
	last4 := key[len(key)-4:]
	row, err := s.users.SetSteamAPIKey(ctx, repository.SetSteamAPIKeyParams{ID: userID, KeyEnc: sealed, KeyLast4: &last4})
	if err != nil {
		return domain.User{}, fmt.Errorf("save api key: %w", err)
	}
	return toDomainUser(row), nil
}

func (s *AuthService) DeleteSteamAPIKey(ctx context.Context, userID int64) (domain.User, error) {
	row, err := s.users.SetSteamAPIKey(ctx, repository.SetSteamAPIKeyParams{ID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("delete api key: %w", err)
	}
	return toDomainUser(row), nil
}
