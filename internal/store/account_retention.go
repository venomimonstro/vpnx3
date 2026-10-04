package store

import "context"

// CleanupExpiredAnonymousTrials removes only abandoned anonymous trial accounts.
// Paid/subscribed users and identified users are excluded even if their current
// entitlement has expired. Device rows cascade with the user.
func (s *Store) CleanupExpiredAnonymousTrials(ctx context.Context)(int64,error){
	tag,err:=s.DB.Exec(ctx,`
		WITH doomed AS (
		  SELECT u.id
		  FROM users u
		  WHERE u.email IS NULL
		    AND u.phone IS NULL
		    AND EXISTS (
		      SELECT 1 FROM devices d
		      WHERE d.user_id=u.id
		    )
		    AND NOT EXISTS (
		      SELECT 1 FROM devices d
		      WHERE d.user_id=u.id
		        AND (
		          d.trial_expires_at IS NULL
		          OR d.trial_expires_at >= now()-interval '30 days'
		        )
		    )
		    AND NOT EXISTS (
		      SELECT 1 FROM subscriptions s
		      WHERE s.user_id=u.id
		    )
		    AND NOT EXISTS (
		      SELECT 1 FROM payments p
		      WHERE p.user_id=u.id
		    )
		  ORDER BY u.created_at
		  LIMIT 500
		  FOR UPDATE OF u SKIP LOCKED
		)
		DELETE FROM users u
		USING doomed d
		WHERE u.id=d.id
	`)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}
