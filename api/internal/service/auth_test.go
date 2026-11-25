package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"testing"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/secrets"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

type fakeUsers struct {
	user  repository.User
	saved repository.SetSteamAPIKeyParams
}

func (f *fakeUsers) UpsertUserBySteamID(context.Context, repository.UpsertUserBySteamIDParams) (repository.User, error) {
	return f.user, nil
}
func (f *fakeUsers) GetUserByID(context.Context, int64) (repository.User, error) { return f.user, nil }
func (f *fakeUsers) SetTradeURL(context.Context, repository.SetTradeURLParams) (repository.User, error) {
	return f.user, nil
}
func (f *fakeUsers) SetSteamAPIKey(_ context.Context, arg repository.SetSteamAPIKeyParams) (repository.User, error) {
	f.saved = arg
	f.user.SteamAPIKeyEnc, f.user.SteamAPIKeyLast4 = arg.KeyEnc, arg.KeyLast4
	return f.user, nil
}

type fakeSteamAuth struct{ validateErr error }

func (fakeSteamAuth) LoginURL(string) string                                   { return "" }
func (fakeSteamAuth) VerifyOpenID(context.Context, url.Values) (string, error) { return "", nil }
func (fakeSteamAuth) GetPlayerSummary(context.Context, string) (steam.PlayerSummary, error) {
	return steam.PlayerSummary{}, nil
}
func (f fakeSteamAuth) ValidateAPIKey(context.Context, string) error { return f.validateErr }

func TestSetSteamAPIKey(t *testing.T) {
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32)))
	const key = "0123456789abcdef0123456789abcdef"
	ctx := context.Background()
	newSvc := func(steamID string, validateErr error) (*AuthService, *fakeUsers) {
		users := &fakeUsers{user: repository.User{ID: 42, SteamID: steamID}}
		return NewAuthService(users, fakeSteamAuth{validateErr: validateErr}, box), users
	}

	t.Run("ok: stored sealed, only last 4 exposed", func(t *testing.T) {
		svc, users := newSvc("76561198000000123", nil)
		u, err := svc.SetSteamAPIKey(ctx, 42, "  "+key+" ")
		if err != nil {
			t.Fatal(err)
		}
		if !u.HasSteamAPIKey || u.SteamAPIKeyLast4 == nil || *u.SteamAPIKeyLast4 != "CDEF" {
			t.Fatalf("user = %+v, want key flag and last4 CDEF", u)
		}
		if bytes.Contains(users.saved.KeyEnc, []byte("0123456789ABCDEF")) {
			t.Fatal("the key must never be stored in plaintext")
		}
		plain, err := box.Open(users.saved.KeyEnc, apiKeyAAD(42))
		if err != nil || string(plain) != "0123456789ABCDEF0123456789ABCDEF" {
			t.Fatalf("sealed key opens to %q, %v", plain, err)
		}
	})

	for _, tt := range []struct {
		name    string
		steamID string
		raw     string
		valErr  error
		want    error
	}{
		{"bad format", "76561198000000123", "not-a-key", nil, domain.ErrInvalidInput},
		{"rejected by steam", "76561198000000123", key, steam.ErrInvalidAPIKey, domain.ErrInvalidAPIKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, users := newSvc(tt.steamID, tt.valErr)
			if _, err := svc.SetSteamAPIKey(ctx, 42, tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if users.saved.KeyEnc != nil {
				t.Fatal("nothing must be saved")
			}
		})
	}

	t.Run("disabled without secrets key", func(t *testing.T) {
		svc := NewAuthService(&fakeUsers{}, fakeSteamAuth{}, nil)
		if _, err := svc.SetSteamAPIKey(ctx, 42, key); !errors.Is(err, domain.ErrFeatureDisabled) {
			t.Fatalf("err = %v, want ErrFeatureDisabled", err)
		}
	})
}
