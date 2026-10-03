package store

import "context"

func (s *Store) CleanupConfigManifests(ctx context.Context) (int64,error) {
	tag,err:=s.DB.Exec(ctx,`
		DELETE FROM config_manifests
		WHERE created_at < now()-interval '30 days'
		  AND version < (
		    SELECT COALESCE(max(version),0)-100
		    FROM config_manifests
		  )
	`)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
