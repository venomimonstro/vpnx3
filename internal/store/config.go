package store

import (
	"context"
	"fmt"
	"time"
)

type ConfigEndpoint struct {
	Kind string `json:"kind"`
	Transport string `json:"transport"`
	Scheme string `json:"scheme"`
	Host string `json:"host"`
	Port int `json:"port"`
	Path string `json:"path,omitempty"`
	Priority int `json:"priority"`
}

type ConfigNode struct {
	ID string `json:"id"`
	Name string `json:"name"`
	CountryCode *string `json:"country_code,omitempty"`
	HealthScore *float64 `json:"health_score,omitempty"`
	Endpoints []ConfigEndpoint `json:"endpoints"`
}

func (s *Store) ActiveConfigNodes(ctx context.Context,role string) ([]ConfigNode,error) {
	rows,err:=s.DB.Query(ctx,`
		SELECT n.id::text,n.name,n.country_code,n.health_score::float8,
		       e.kind,e.transport,e.scheme,e.host,e.port,e.path,e.priority
		FROM nodes n
		JOIN node_endpoints e ON e.node_id=n.id AND e.enabled=true
		WHERE n.role=$1::node_role AND n.status='active'
		ORDER BY n.country_code NULLS LAST,n.name,e.priority,e.id
	`,role)
	if err!=nil { return nil,fmt.Errorf("list active config nodes: %w",err) }
	defer rows.Close()
	out:=make([]ConfigNode,0)
	index:=map[string]int{}
	for rows.Next() {
		var id,name string
		var country *string
		var health *float64
		var ep ConfigEndpoint
		if err:=rows.Scan(&id,&name,&country,&health,&ep.Kind,&ep.Transport,&ep.Scheme,&ep.Host,&ep.Port,&ep.Path,&ep.Priority); err!=nil { return nil,err }
		i,ok:=index[id]
		if !ok {
			i=len(out)
			index[id]=i
			out=append(out,ConfigNode{ID:id,Name:name,CountryCode:country,HealthScore:health,Endpoints:[]ConfigEndpoint{}})
		}
		out[i].Endpoints=append(out[i].Endpoints,ep)
	}
	return out,rows.Err()
}

func (s *Store) NextConfigVersion(ctx context.Context) (int64,error) {
	var version int64
	err:=s.DB.QueryRow(ctx,"SELECT nextval('config_manifest_version_seq')").Scan(&version)
	return version,err
}

type StoredManifest struct {
	Version int64
	Payload []byte
	Signature []byte
	KeyID string
	CreatedAt time.Time
}

func (s *Store) SaveConfigManifest(ctx context.Context,version int64,payload,signature []byte,keyID,adminID string) error {
	_,err:=s.DB.Exec(ctx,`
		INSERT INTO config_manifests(version,payload,payload_raw,signature,key_id,created_by)
		VALUES($1,$2::jsonb,$3,$4,$5,$6)
	`,version,string(payload),payload,signature,keyID,adminID)
	if err!=nil { return fmt.Errorf("save config manifest: %w",err) }
	return nil
}

func (s *Store) LatestConfigManifest(ctx context.Context) (StoredManifest,error) {
	var m StoredManifest
	err:=s.DB.QueryRow(ctx,`
		SELECT version,payload_raw,signature,key_id,created_at
		FROM config_manifests WHERE payload_raw IS NOT NULL
		ORDER BY version DESC LIMIT 1
	`).Scan(&m.Version,&m.Payload,&m.Signature,&m.KeyID,&m.CreatedAt)
	return m,err
}
