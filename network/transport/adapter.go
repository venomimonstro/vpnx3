package transport

import (
	"context"
	"time"
)

type SessionRequest struct {
	SessionID       string
	DeviceID        string
	ClientPublicKey string
	AssignedIP      string
	ExpiresAt       time.Time
}

type SessionConfig struct {
	Transport       string    `json:"transport"`
	AssignedIP      string    `json:"assigned_ip"`
	ServerPublicKey string    `json:"server_public_key"`
	Endpoint        string    `json:"endpoint"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type Adapter interface {
	Name() string
	CreateSession(context.Context,SessionRequest) (SessionConfig,error)
	CloseSession(context.Context,string) error
	Healthy(context.Context) error
}
