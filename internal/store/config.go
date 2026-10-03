package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ConfigIngress struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	CountryCode *string  `json:"country_code,omitempty"`
	PublicIP    *string  `json:"public_ip,omitempty"`
	HealthScore *float64 `json:"health_score,omitempty"`
}

func (s *Store) ActiveIngresses(ctx context.Context) ([]ConfigIngress,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT id::text,name,country_code,host(public_ip),health_score::float8
		FROM nodes
		WHERE role='ingress' AND status='active' AND public_ip IS NOT NULL
		ORDER BY country_code NULLS LAST,name
	`)
	if err!=nil { return nil,fmt.Errorf("list active ingresses: %w",err) }
	defer rows.Close()
	out:=make([]ConfigIngress,0)
	for rows.Next() {
		var n ConfigIngress
		if err:=rows.Scan(&n.ID,&n.Name,&n.CountryCode,&n.PublicIP,&n.HealthScore); err!=nil { return nil,err }
		out=append(out,n)
	}
	return out,rows.Err()
}

func (s *Store) NextConfigVersion(ctx context.Context) (int64,error) {
	var version int64
	err:=s.DB.QueryRow(ctx,"SELECT nextval('config_manifest_version_seq')").Scan(&version)
	return version,err
}

type StoredManifest struct {
	Version   int64           `json:"version"`
	Payload   json.RawMessage `json:"payload"`
	Signature []byte          `json:"-"`
	KeyID     string          `json:"key_id"`
	CreatedAt time.Time       `json:"created_at"`
}

func (s *Store) SaveConfigManifest(ctx context.Context,version int64,payload,signature []byte,keyID,adminID string) error {
	_,err:=s.DB.Exec(ctx,`
		INSERT INTO config_manifests(version,payload,signature,key_id,created_by)
		VALUES($1,$2::jsonb,$3,$4,$5)
	`,version,string(payload),signature,keyID,adminID)
	if err!=nil { return fmt.Errorf("save config manifest: %w",err) }
	return nil
}

func (s *Store) LatestConfigManifest(ctx context.Context) (StoredManifest,error) {
	var m StoredManifest
	var raw string
	err:=s.DB.QueryRow(ctx,`
		SELECT version,payload::text,signature,key_id,created_at
		FROM config_manifests ORDER BY version DESC LIMIT 1
	`).Scan(&m.Version,&raw,&m.Signature,&m.KeyID,&m.CreatedAt)
	if err!=nil { return StoredManifest{},err }
	m.Payload=json.RawMessage(raw)
	return m,nil
}
