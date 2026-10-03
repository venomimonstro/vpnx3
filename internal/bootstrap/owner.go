package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/venomimonstro/vpnx3/internal/adminauth"
	"github.com/venomimonstro/vpnx3/internal/store"
)

func EnsureOwner(ctx context.Context, s *store.Store, logger *slog.Logger, email, password string) error {
	count, err := s.AdminCount(ctx)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count > 0 {
		return nil
	}
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return fmt.Errorf("no administrator exists: set VPNX3_BOOTSTRAP_OWNER_EMAIL and VPNX3_BOOTSTRAP_OWNER_PASSWORD for first startup")
	}
	hash, err := adminauth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap owner password: %w", err)
	}
	id, err := s.CreateOwner(ctx, email, hash)
	if err != nil {
		return err
	}
	logger.Warn("bootstrap owner created; remove bootstrap password from environment", "admin_id", id, "email", email)
	return nil
}
