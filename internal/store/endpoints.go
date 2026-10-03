package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type NodeEndpoint struct {
	ID string `json:"id"`
	NodeID string `json:"node_id"`
	Kind string `json:"kind"`
	Transport string `json:"transport"`
	Scheme string `json:"scheme"`
	Host string `json:"host"`
	Port int `json:"port"`
	Path string `json:"path,omitempty"`
	Priority int `json:"priority"`
	Enabled bool `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateNodeEndpointInput struct {
	NodeID string
	Kind string
	Transport string
	Scheme string
	Host string
	Port int
	Path string
	Priority int
}

func validateNodeEndpoint(in *CreateNodeEndpointInput) error {
	in.Kind=strings.ToLower(strings.TrimSpace(in.Kind))
	in.Transport=strings.ToLower(strings.TrimSpace(in.Transport))
	in.Scheme=strings.ToLower(strings.TrimSpace(in.Scheme))
	in.Host=strings.TrimSpace(in.Host)
	in.Path=strings.TrimSpace(in.Path)
	if in.Transport=="" || len(in.Transport)>64 { return fmt.Errorf("invalid transport") }
	if in.Host=="" || len(in.Host)>255 || strings.ContainsAny(in.Host," \t\r\n") { return fmt.Errorf("invalid host") }
	if in.Port<1 || in.Port>65535 { return fmt.Errorf("invalid port") }
	if in.Path!="" && !strings.HasPrefix(in.Path,"/") { return fmt.Errorf("invalid path") }
	if in.Priority==0 { in.Priority=100 }
	if in.Priority<0 || in.Priority>10000 { return fmt.Errorf("invalid priority") }
	switch in.Kind {
	case "session_api":
		if in.Scheme!="https" { return fmt.Errorf("session_api requires https") }
	case "wireguard":
		if in.Scheme!="udp" || in.Transport!="wireguard" { return fmt.Errorf("wireguard endpoint requires udp/wireguard") }
	case "ingress":
		if in.Scheme!="https" { return fmt.Errorf("ingress endpoint requires https") }
	default:
		return fmt.Errorf("invalid endpoint kind")
	}
	return nil
}

func (s *Store) CreateNodeEndpoint(ctx context.Context,in CreateNodeEndpointInput) (NodeEndpoint,error) {
	if err:=validateNodeEndpoint(&in); err!=nil { return NodeEndpoint{},err }
	var role string
	if err:=s.DB.QueryRow(ctx,"SELECT role::text FROM nodes WHERE id=$1",in.NodeID).Scan(&role); err!=nil {
		if errors.Is(err,pgx.ErrNoRows) { return NodeEndpoint{},ErrNotFound }
		return NodeEndpoint{},err
	}
	if (in.Kind=="wireguard" || in.Kind=="session_api") && role!="worker" {
		return NodeEndpoint{},fmt.Errorf("worker endpoint requires worker node")
	}
	if in.Kind=="ingress" && role!="ingress" { return NodeEndpoint{},fmt.Errorf("ingress endpoint requires ingress node") }

	var ep NodeEndpoint
	err:=s.DB.QueryRow(ctx,`
		INSERT INTO node_endpoints(node_id,kind,transport,scheme,host,port,path,priority)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id::text,node_id::text,kind,transport,scheme,host,port,path,priority,enabled,created_at,updated_at
	`,in.NodeID,in.Kind,in.Transport,in.Scheme,in.Host,in.Port,in.Path,in.Priority).
		Scan(&ep.ID,&ep.NodeID,&ep.Kind,&ep.Transport,&ep.Scheme,&ep.Host,&ep.Port,&ep.Path,&ep.Priority,&ep.Enabled,&ep.CreatedAt,&ep.UpdatedAt)
	if err!=nil { return NodeEndpoint{},fmt.Errorf("create node endpoint: %w",err) }
	return ep,nil
}

func (s *Store) ListNodeEndpoints(ctx context.Context,nodeID string) ([]NodeEndpoint,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,node_id::text,kind,transport,scheme,host,port,path,priority,enabled,created_at,updated_at
		FROM node_endpoints WHERE node_id=$1 ORDER BY priority,id
	`,nodeID)
	if err!=nil { return nil,err }
	defer rows.Close()
	out:=make([]NodeEndpoint,0)
	for rows.Next() {
		var ep NodeEndpoint
		if err:=rows.Scan(&ep.ID,&ep.NodeID,&ep.Kind,&ep.Transport,&ep.Scheme,&ep.Host,&ep.Port,&ep.Path,&ep.Priority,&ep.Enabled,&ep.CreatedAt,&ep.UpdatedAt); err!=nil { return nil,err }
		out=append(out,ep)
	}
	return out,rows.Err()
}

func (s *Store) DeleteNodeEndpoint(ctx context.Context,nodeID,endpointID string) error {
	tag,err:=s.DB.Exec(ctx,"DELETE FROM node_endpoints WHERE id=$1 AND node_id=$2",endpointID,nodeID)
	if err!=nil { return err }
	if tag.RowsAffected()!=1 { return ErrNotFound }
	return nil
}
