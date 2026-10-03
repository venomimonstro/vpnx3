package store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	DB *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store { return &Store{DB: db} }

type Admin struct {
	ID           string
	Email        string
	PasswordHash string
	Status       string
	Permissions  []string
}

func (s *Store) AdminByEmail(ctx context.Context, email string) (Admin, error) {
	var a Admin
	err := s.DB.QueryRow(ctx, `
		SELECT id::text, email, COALESCE(password_hash,''), status
		FROM admin_users WHERE lower(email)=lower($1)
	`, email).Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("admin by email: %w", err)
	}
	a.Permissions, err = s.adminPermissions(ctx, a.ID)
	return a, err
}

func (s *Store) AdminBySessionHash(ctx context.Context, hash []byte) (Admin, error) {
	var a Admin
	err := s.DB.QueryRow(ctx, `
		SELECT u.id::text, u.email, COALESCE(u.password_hash,''), u.status
		FROM admin_sessions s
		JOIN admin_users u ON u.id=s.admin_user_id
		WHERE s.token_hash=$1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
		  AND u.status='active'
	`, hash).Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("admin by session: %w", err)
	}
	a.Permissions, err = s.adminPermissions(ctx, a.ID)
	return a, err
}

func (s *Store) adminPermissions(ctx context.Context, adminID string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT DISTINCT p.code
		FROM permissions p
		JOIN role_permissions rp ON rp.permission_id=p.id
		JOIN admin_user_roles aur ON aur.role_id=rp.role_id
		WHERE aur.admin_user_id=$1
		ORDER BY p.code
	`, adminID)
	if err != nil {
		return nil, fmt.Errorf("admin permissions: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, rows.Err()
}

func (s *Store) AdminCount(ctx context.Context) (int, error) {
	var count int
	err := s.DB.QueryRow(ctx, "SELECT count(*) FROM admin_users").Scan(&count)
	return count, err
}

func (s *Store) CreateOwner(ctx context.Context, email, passwordHash string) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil { return "", err }
	defer tx.Rollback(ctx)

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO admin_users(email,password_hash,status)
		VALUES(lower($1),$2,'active') RETURNING id::text
	`, email, passwordHash).Scan(&id); err != nil {
		return "", fmt.Errorf("create owner: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admin_user_roles(admin_user_id,role_id)
		SELECT $1,id FROM roles WHERE code='owner'
	`, id); err != nil {
		return "", fmt.Errorf("assign owner role: %w", err)
	}
	if err := tx.Commit(ctx); err != nil { return "", err }
	return id, nil
}

func (s *Store) CreateAdminSession(ctx context.Context, adminID string, tokenHash []byte, sourceIP net.IP, userAgent string, expires time.Time) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO admin_sessions(admin_user_id,token_hash,source_ip,user_agent,expires_at)
		VALUES($1,$2,$3,$4,$5)
	`, adminID, tokenHash, sourceIP, userAgent, expires)
	return err
}

func (s *Store) RevokeAdminSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE admin_sessions SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}

func (s *Store) TouchAdminSession(ctx context.Context, tokenHash []byte) {
	_, _ = s.DB.Exec(ctx, `UPDATE admin_sessions SET last_seen_at=now() WHERE token_hash=$1`, tokenHash)
}

func (s *Store) WriteAudit(ctx context.Context, actorType, actorID, action, resourceType, resourceID, requestID, sourceIP, result string) error {
	_, err := s.DB.Exec(ctx, `
		INSERT INTO audit_log(actor_type,actor_id,action,resource_type,resource_id,request_id,source_ip,result)
		VALUES($1,NULLIF($2,''),$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,'')::inet,$8)
	`, actorType, actorID, action, resourceType, resourceID, requestID, sourceIP, result)
	return err
}
