package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/venomimonstro/vpnx3/internal/accesslease"
	"github.com/venomimonstro/vpnx3/internal/ipam"
	"github.com/venomimonstro/vpnx3/internal/revocations"
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

type persistedSession struct {
	ID              string                  `json:"id"`
	DeviceID        string                  `json:"device_id"`
	ClientPublicKey string                  `json:"client_public_key"`
	AssignedIP      string                  `json:"assigned_ip"`
	ExpiresAt       time.Time               `json:"expires_at"`
	Config          transport.SessionConfig `json:"config"`
}

type Manager struct {
	opMu      sync.Mutex
	mu        sync.RWMutex
	verifier  *accesslease.Verifier
	ipam      *ipam.Pool
	adapter   transport.Adapter
	statePath string
	sessions  map[string]Session
	byDevice  map[string]string
}

func New(verifier *accesslease.Verifier,pool *ipam.Pool,adapter transport.Adapter,statePath string) *Manager {
	return &Manager{
		verifier:verifier,ipam:pool,adapter:adapter,statePath:statePath,
		sessions:map[string]Session{},byDevice:map[string]string{},
	}
}

func (m *Manager) Restore(ctx context.Context,now time.Time) (int,error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if m.statePath=="" { return 0,nil }
	raw,err:=os.ReadFile(m.statePath)
	if os.IsNotExist(err) { return 0,nil }
	if err!=nil { return 0,fmt.Errorf("read session state: %w",err) }

	var stored []persistedSession
	if err:=json.Unmarshal(raw,&stored); err!=nil {
		return 0,fmt.Errorf("decode session state: %w",err)
	}

	restored:=0
	for _,p:=range stored {
		if !p.ExpiresAt.After(now) { continue }
		if _,err:=m.ipam.Reserve(p.DeviceID,p.AssignedIP); err!=nil {
			return restored,fmt.Errorf("restore address for %s: %w",p.DeviceID,err)
		}
		cfg,err:=m.adapter.CreateSession(ctx,transport.SessionRequest{
			SessionID:p.ID,DeviceID:p.DeviceID,ClientPublicKey:p.ClientPublicKey,
			AssignedIP:p.AssignedIP,ExpiresAt:p.ExpiresAt,
		})
		if err!=nil {
			m.ipam.Release(p.DeviceID)
			return restored,fmt.Errorf("restore transport session %s: %w",p.ID,err)
		}
		s:=Session{
			ID:p.ID,DeviceID:p.DeviceID,ClientPublicKey:p.ClientPublicKey,
			AssignedIP:p.AssignedIP,ExpiresAt:p.ExpiresAt,Config:cfg,
		}
		m.mu.Lock()
		m.sessions[s.ID]=s
		m.byDevice[s.DeviceID]=s.ID
		m.mu.Unlock()
		restored++
	}
	if err:=m.persist(); err!=nil { return restored,err }
	return restored,nil
}

func (m *Manager) Start(ctx context.Context,env accesslease.Envelope,clientPublicKey string,now time.Time) (Session,error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	claims,err:=m.verifier.Verify(env,now)
	if err!=nil { return Session{},err }
	if clientPublicKey=="" { return Session{},fmt.Errorf("client public key is required") }
	if claims.TunnelPublicKey != clientPublicKey {
		return Session{},fmt.Errorf("access lease is bound to a different tunnel public key")
	}

	m.mu.RLock()
	existingID,hasExisting:=m.byDevice[claims.DeviceID]
	existing:=m.sessions[existingID]
	m.mu.RUnlock()

	if hasExisting && existing.ExpiresAt.After(now) && existing.ClientPublicKey==clientPublicKey {
		return existing,nil
	}
	if hasExisting {
		if err:=m.closeLocked(ctx,existingID); err!=nil {
			return Session{},fmt.Errorf("close previous session: %w",err)
		}
	}

	addr,err:=m.ipam.Acquire(claims.DeviceID)
	if err!=nil { return Session{},err }

	cfg,err:=m.adapter.CreateSession(ctx,transport.SessionRequest{
		SessionID:claims.LeaseID,DeviceID:claims.DeviceID,
		ClientPublicKey:clientPublicKey,AssignedIP:addr.String(),ExpiresAt:claims.ExpiresAt,
	})
	if err!=nil {
		m.ipam.Release(claims.DeviceID)
		return Session{},err
	}

	session:=Session{
		ID:claims.LeaseID,DeviceID:claims.DeviceID,ClientPublicKey:clientPublicKey,
		AssignedIP:addr.String(),ExpiresAt:claims.ExpiresAt,Config:cfg,
	}
	m.mu.Lock()
	m.sessions[session.ID]=session
	m.byDevice[session.DeviceID]=session.ID
	m.mu.Unlock()
	if err:=m.persist(); err!=nil {
		_ = m.closeLocked(ctx,session.ID)
		return Session{},fmt.Errorf("persist session: %w",err)
	}
	return session,nil
}

func (m *Manager) Close(ctx context.Context,sessionID string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.closeLocked(ctx,sessionID)
}

func (m *Manager) closeLocked(ctx context.Context,sessionID string) error {
	m.mu.RLock()
	session,ok:=m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok { return nil }

	if err:=m.adapter.CloseSession(ctx,session.ClientPublicKey); err!=nil {
		return err
	}

	m.mu.Lock()
	delete(m.sessions,sessionID)
	if current,exists:=m.byDevice[session.DeviceID]; exists && current==sessionID {
		delete(m.byDevice,session.DeviceID)
	}
	m.mu.Unlock()
	m.ipam.Release(session.DeviceID)
	return m.persist()
}

func (m *Manager) Sweep(ctx context.Context,now time.Time) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.mu.RLock()
	var expired []string
	for id,s:=range m.sessions {
		if !s.ExpiresAt.After(now) { expired=append(expired,id) }
	}
	m.mu.RUnlock()

	for _,id:=range expired { _=m.closeLocked(ctx,id) }
}

func (m *Manager) CloseRevokedDeviceHashes(ctx context.Context,hashes []string)(int,error){
	if len(hashes)==0{return 0,nil}
	set:=make(map[string]struct{},len(hashes))
	for _,hash:=range hashes{set[hash]=struct{}{}}

	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	ids:=make([]string,0)
	for id,s:=range m.sessions{
		if _,ok:=set[revocations.DeviceHash(s.DeviceID)];ok{ids=append(ids,id)}
	}
	m.mu.RUnlock()

	closed:=0
	for _,id:=range ids{
		if err:=m.closeLocked(ctx,id);err!=nil{return closed,err}
		closed++
	}
	return closed,nil
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

func (m *Manager) persist() error {
	if m.statePath=="" { return nil }

	m.mu.RLock()
	stored:=make([]persistedSession,0,len(m.sessions))
	for _,s:=range m.sessions {
		stored=append(stored,persistedSession{
			ID:s.ID,DeviceID:s.DeviceID,ClientPublicKey:s.ClientPublicKey,
			AssignedIP:s.AssignedIP,ExpiresAt:s.ExpiresAt,Config:s.Config,
		})
	}
	m.mu.RUnlock()

	raw,err:=json.Marshal(stored)
	if err!=nil { return fmt.Errorf("encode session state: %w",err) }
	if err:=os.MkdirAll(filepath.Dir(m.statePath),0700); err!=nil { return err }
	tmp:=m.statePath+".tmp"
	if err:=os.WriteFile(tmp,raw,0600); err!=nil { return err }
	if err:=os.Rename(tmp,m.statePath); err!=nil { return err }
	return nil
}
