package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"

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
}

type SteamAuth interface {
	LoginURL(state string) string
	VerifyOpenID(ctx context.Context, q url.Values) (string, error)
	GetPlayerSummary(ctx context.Context, steamID string) (steam.PlayerSummary, error)
}

type AuthService struct {
	users UserRepository
	steam SteamAuth
}

func NewAuthService(users UserRepository, steam SteamAuth) *AuthService {
	return &AuthService{users: users, steam: steam}
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
		ID:           u.ID,
		SteamID:      u.SteamID,
		DisplayName:  u.DisplayName,
		AvatarURL:    u.AvatarURL,
		BalanceCents: u.BalanceCents,
		TradeURL:     u.TradeURL,
		CreatedAt:    u.CreatedAt,
	}
}
