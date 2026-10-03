package wireguard

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/venomimonstro/vpnx3/network/transport"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Adapter struct {
	interfaceName string
	endpoint      string
}

func New(interfaceName, endpoint string) (*Adapter, error) {
	if strings.TrimSpace(interfaceName) == "" {
		return nil, fmt.Errorf("wireguard interface is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("wireguard endpoint is required")
	}
	return &Adapter{interfaceName: interfaceName, endpoint: endpoint}, nil
}

func (a *Adapter) Name() string { return "wireguard" }

func (a *Adapter) CreateSession(ctx context.Context, req transport.SessionRequest) (transport.SessionConfig, error) {
	select {
	case <-ctx.Done():
		return transport.SessionConfig{}, ctx.Err()
	default:
	}

	key, err := wgtypes.ParseKey(req.ClientPublicKey)
	if err != nil {
		return transport.SessionConfig{}, fmt.Errorf("parse client wireguard key: %w", err)
	}
	ip := net.ParseIP(req.AssignedIP)
	if ip == nil || ip.To4() == nil {
		return transport.SessionConfig{}, fmt.Errorf("invalid assigned IPv4 address")
	}
	allowed := net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)}

	client, err := wgctrl.New()
	if err != nil {
		return transport.SessionConfig{}, fmt.Errorf("open wgctrl: %w", err)
	}
	defer client.Close()

	device, err := client.Device(a.interfaceName)
	if err != nil {
		return transport.SessionConfig{}, fmt.Errorf("read wireguard device: %w", err)
	}

	cfg := wgtypes.Config{Peers: []wgtypes.PeerConfig{{
		PublicKey:         key,
		Remove:            false,
		ReplaceAllowedIPs: true,
		AllowedIPs:        []net.IPNet{allowed},
	}}}

	if err := client.ConfigureDevice(a.interfaceName, cfg); err != nil {
		return transport.SessionConfig{}, fmt.Errorf("configure wireguard peer: %w", err)
	}
	return transport.SessionConfig{
		Transport:       a.Name(),
		AssignedIP:      req.AssignedIP,
		ServerPublicKey: device.PublicKey.String(),
		Endpoint:        a.endpoint,
		ExpiresAt:       req.ExpiresAt,
	}, nil
}

func (a *Adapter) CloseSession(ctx context.Context, clientPublicKey string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	key, err := wgtypes.ParseKey(clientPublicKey)
	if err != nil {
		return fmt.Errorf("parse client wireguard key: %w", err)
	}
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("open wgctrl: %w", err)
	}
	defer client.Close()

	if err := client.ConfigureDevice(a.interfaceName, wgtypes.Config{Peers: []wgtypes.PeerConfig{{
		PublicKey: key,
		Remove:    true,
	}}}); err != nil {
		return fmt.Errorf("remove wireguard peer: %w", err)
	}
	return nil
}

func (a *Adapter) Healthy(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("open wgctrl: %w", err)
	}
	defer client.Close()

	if _, err := client.Device(a.interfaceName); err != nil {
		return fmt.Errorf("read wireguard device: %w", err)
	}
	return nil
}
