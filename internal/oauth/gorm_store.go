package oauth

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// OAuthAccountModel is the persistent model for OAuth-linked accounts.
type OAuthAccountModel struct {
	ID             uint      `gorm:"primaryKey"`
	Provider       string    `gorm:"size:64;not null;index:idx_oauth_provider_uid,unique"`
	ProviderUserID string    `gorm:"size:191;not null;index:idx_oauth_provider_uid,unique"`
	Email          string    `gorm:"size:191"`
	AccessToken    string    `gorm:"type:text;not null"`
	RefreshToken   string    `gorm:"type:text"`
	IDToken        string    `gorm:"type:text"`
	TokenExpiresAt time.Time `gorm:"index"`
	ClientID       string    `gorm:"size:191"`
	TokenEndpoint  string    `gorm:"size:512"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (OAuthAccountModel) TableName() string { return "oauth_accounts" }

// GormOAuthAccountStore persists OAuth account links with GORM.
type GormOAuthAccountStore struct {
	db *gorm.DB
}

func NewSQLiteOAuthAccountStore(dsn string) (*GormOAuthAccountStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty sqlite dsn")
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if err := db.AutoMigrate(&OAuthAccountModel{}); err != nil {
		return nil, fmt.Errorf("auto migrate oauth_accounts: %w", err)
	}
	return &GormOAuthAccountStore{db: db}, nil
}

func (s *GormOAuthAccountStore) Upsert(ctx context.Context, account OAuthAccount) error {
	if account.Provider == "" || account.ProviderUserID == "" {
		return fmt.Errorf("provider and provider_user_id are required")
	}

	model := OAuthAccountModel{
		Provider:       account.Provider,
		ProviderUserID: account.ProviderUserID,
		Email:          account.Email,
		AccessToken:    account.AccessToken,
		RefreshToken:   account.RefreshToken,
		IDToken:        account.IDToken,
		TokenExpiresAt: account.TokenExpiresAt,
		ClientID:       account.ClientID,
		TokenEndpoint:  account.TokenEndpoint,
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing OAuthAccountModel
		err := tx.Where("provider = ? AND provider_user_id = ?", account.Provider, account.ProviderUserID).First(&existing).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return tx.Create(&model).Error
			}
			return err
		}

		existing.Email = model.Email
		existing.AccessToken = model.AccessToken
		existing.RefreshToken = model.RefreshToken
		existing.IDToken = model.IDToken
		existing.TokenExpiresAt = model.TokenExpiresAt
		existing.ClientID = model.ClientID
		existing.TokenEndpoint = model.TokenEndpoint
		return tx.Save(&existing).Error
	})
}

func (s *GormOAuthAccountStore) GetLatestByProvider(ctx context.Context, provider string) (OAuthAccount, bool, error) {
	if provider == "" {
		return OAuthAccount{}, false, fmt.Errorf("provider is required")
	}

	var model OAuthAccountModel
	err := s.db.WithContext(ctx).
		Where("provider = ?", provider).
		Order("updated_at DESC").
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return OAuthAccount{}, false, nil
		}
		return OAuthAccount{}, false, err
	}

	return OAuthAccount{
		Provider:       model.Provider,
		ProviderUserID: model.ProviderUserID,
		Email:          model.Email,
		AccessToken:    model.AccessToken,
		RefreshToken:   model.RefreshToken,
		IDToken:        model.IDToken,
		TokenExpiresAt: model.TokenExpiresAt,
		ClientID:       model.ClientID,
		TokenEndpoint:  model.TokenEndpoint,
		UpdatedAt:      model.UpdatedAt,
	}, true, nil
}
