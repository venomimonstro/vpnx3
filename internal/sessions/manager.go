package sessions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/ipam"
	"github.com/venomimonstro/vpnx3/network/transport"
)

type Session struct {
	ID              string                  `json:"id"`
	DeviceID        string                  `json:"device_id"`
	ClientPublicKey string                  `json:"-"`
	AssignedIP      string                  `json:"assigned_ip"`
	ExpiresAt       time.Time               `json:"expires_at"`
	Config          transport.SessionConfig `json:"config"`
}

type Manager struct {
	mu       sync.Mutex
	verifier *accesslease.Verifier
	ipam     *ipam.Pool
	adapter  transport.Adapter
	sessions map[string]Session
	byDevice map[string]string
}

func New(verifier *accesslease.Verifier, pool *ipam.Pool, adapter transport.Adapter) *Manager {
	return &Manager{
		verifier: verifier,
		ipam:     pool,
		adapter:  adapter,
		sessions: map[string]Session{},
		byDevice: map[string]string{},
	}
}

func (m *Manager) Start(ctx context.Context, env accesslease.Envelope, clientPublicKey string, now time.Time) (Session, error) {
	claims, err := m.verifier.Verify(env, now)
	if err != nil {
		return Session{}, err
	}
	if clientPublicKey == "" {
		return Session{}, fmt.Errorf("client public key is required")
	}

	var oldSessionID string
	m.mu.Lock()
	if existingID, ok := m.byDevice[claims.DeviceID]; ok {
		existing := m.sessions[existingID]
		if existing.ExpiresAt.After(now) && existing.ClientPublicKey == clientPublicKey {
			m.mu.Unlock()
			return existing, nil
		}
		oldSessionID = existingID
	}
	m.mu.Unlock()

	// Устройство может иметь только одну активную сессию на worker.
	// Старый peer удаляется до создания нового, чтобы не получить два AllowedIP /32
	// на разных WireGuard-ключах.
	if oldSessionID != "" {
		if err := m.Close(ctx, oldSessionID); err != nil {
			return Session{}, fmt.Errorf("close previous session: %w", err)
		}
	}

	addr, err := m.ipam.Acquire(claims.DeviceID)
	if err != nil {
		return Session{}, err
	}

	cfg, err := m.adapter.CreateSession(ctx, transport.SessionRequest{
		SessionID:       claims.LeaseID,
		DeviceID:        claims.DeviceID,
		ClientPublicKey: clientPublicKey,
		AssignedIP:      addr.String(),
		ExpiresAt:       claims.ExpiresAt,
	})
	if err != nil {
		m.ipam.Release(claims.DeviceID)
		return Session{}, err
	}

	session := Session{
		ID:              claims.LeaseID,
		DeviceID:        claims.DeviceID,
		ClientPublicKey: clientPublicKey,
		AssignedIP:      addr.String(),
		ExpiresAt:       claims.ExpiresAt,
		Config:          cfg,
	}
	m.mu.Lock()
	m.sessions[session.ID] = session
	m.byDevice[session.DeviceID] = session.ID
	m.mu.Unlock()
	return session, nil
}

func (m *Manager) Close(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
		if current, exists := m.byDevice[session.DeviceID]; exists && current == sessionID {
			delete(m.byDevice, session.DeviceID)
		}
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}

	if err := m.adapter.CloseSession(ctx, session.ClientPublicKey); err != nil {
		// Локальную запись уже удалили, но адрес нельзя переиспользовать,
		// пока peer потенциально остался в транспортном слое.
		return err
	}
	m.ipam.Release(session.DeviceID)
	return nil
}

func (m *Manager) Sweep(ctx context.Context, now time.Time) {
	m.mu.Lock()
	var expired []string
	for id, s := range m.sessions {
		if !s.ExpiresAt.After(now) {
			expired = append(expired, id)
		}
	}
	m.mu.Unlock()

	for _, id := range expired {
		_ = m.Close(ctx, id)
	}
}

func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}
