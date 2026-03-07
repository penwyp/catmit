package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/oauth"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newOAuthOpenAICommand())
}

func newOAuthOpenAICommand() *cobra.Command {
	var listenAddr string
	var stateTTL time.Duration

	cmd := &cobra.Command{
		Use:   "oauth-openai",
		Short: "Start local OpenAI OAuth callback server",
		Long:  "Start local HTTP endpoints for OpenAI OAuth login flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := oauth.LoadOpenAIConfigFromEnv()
			if err := cfg.Validate(); err != nil {
				return errors.Wrap(errors.ErrTypeConfig, "invalid OAuth config", err)
			}

			provider, err := oauth.NewOpenAIProvider(cfg)
			if err != nil {
				return errors.Wrap(errors.ErrTypeConfig, "failed to initialize OAuth provider", err)
			}

			accountStore := oauth.OAuthAccountStore(oauth.NewMemoryOAuthAccountStore())
			if sqlitePath := os.Getenv("CATMIT_OAUTH_DB_SQLITE_PATH"); sqlitePath != "" {
				accountStore, err = oauth.NewSQLiteOAuthAccountStore(sqlitePath)
				if err != nil {
					return errors.Wrap(errors.ErrTypeConfig, "failed to initialize oauth sqlite store", err)
				}
			}

			handler := oauth.NewHandler(
				oauth.HandlerConfig{StateTTL: stateTTL},
				provider,
				oauth.NewMemoryStateStore(),
				accountStore,
				oauth.NewIDTokenVerifier(provider.OIDCMode()),
			)
			mux := http.NewServeMux()
			handler.Register(mux)

			srv := &http.Server{
				Addr:              listenAddr,
				Handler:           mux,
				ReadHeaderTimeout: 5 * time.Second,
			}

			fmt.Fprintf(cmd.OutOrStdout(), "OAuth server listening on %s\n", listenAddr)
			fmt.Fprintf(cmd.OutOrStdout(), "Start URL: http://%s%s\n", listenAddr, oauth.OpenAIStartPath)
			fmt.Fprintf(cmd.OutOrStdout(), "Callback URL (configure in OpenAI app): %s\n", provider.RedirectURI())
			fmt.Fprintf(cmd.OutOrStdout(), "OIDC mode: %s\n", provider.OIDCMode())
			if sqlitePath := os.Getenv("CATMIT_OAUTH_DB_SQLITE_PATH"); sqlitePath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "OAuth account store: sqlite (%s, auto-migrate enabled)\n", sqlitePath)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "OAuth account store: memory (set CATMIT_OAUTH_DB_SQLITE_PATH to persist)")
			}

			errCh := make(chan error, 1)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
				close(errCh)
			}()

			select {
			case <-cmd.Context().Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					return errors.Wrap(errors.ErrTypeExternal, "oauth server shutdown failed", err)
				}
				return nil
			case err := <-errCh:
				if err == nil {
					return nil
				}
				return errors.Wrap(errors.ErrTypeExternal, "oauth server failed", err)
			}
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", "127.0.0.1:8085", "HTTP listen address")
	cmd.Flags().DurationVar(&stateTTL, "state-ttl", 5*time.Minute, "OAuth state lifetime")
	return cmd
}
