package oauth

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteOAuthAccountStore_Upsert(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "oauth.db")
	store, err := NewSQLiteOAuthAccountStore(dsn)
	if err != nil {
		t.Fatalf("NewSQLiteOAuthAccountStore() error = %v", err)
	}

	ctx := context.Background()
	err = store.Upsert(ctx, OAuthAccount{
		Provider:       "openai",
		ProviderUserID: "u1",
		Email:          "u1@example.com",
		AccessToken:    "a1",
		RefreshToken:   "r1",
		IDToken:        "i1",
		TokenExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Upsert(insert) error = %v", err)
	}

	err = store.Upsert(ctx, OAuthAccount{
		Provider:       "openai",
		ProviderUserID: "u1",
		Email:          "u2@example.com",
		AccessToken:    "a2",
	})
	if err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}

	var rows int64
	if err := store.db.Model(&OAuthAccountModel{}).Where("provider = ? AND provider_user_id = ?", "openai", "u1").Count(&rows).Error; err != nil {
		t.Fatalf("count error = %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}

	var model OAuthAccountModel
	if err := store.db.Where("provider = ? AND provider_user_id = ?", "openai", "u1").First(&model).Error; err != nil {
		t.Fatalf("query error = %v", err)
	}
	if model.Email != "u2@example.com" || model.AccessToken != "a2" {
		t.Fatalf("unexpected row after update: %+v", model)
	}
}
