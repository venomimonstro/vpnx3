package store

import (
	"context"
	"fmt"
	"time"
)

type OperationalIssue struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Title string `json:"title"`
	Detail string `json:"detail"`
	ResourceType string `json:"resource_type"`
	ResourceID string `json:"resource_id"`
	ObservedAt time.Time `json:"observed_at"`
}

func (s *Store) OperationalIssues(ctx context.Context,limit int)([]OperationalIssue,error){
	if limit<=0||limit>500{limit=200}
	rows,err:=s.DB.Query(ctx,`
		WITH issues AS (
		  SELECT
		    CASE WHEN n.status='quarantined' THEN 'critical' ELSE 'warning' END AS severity,
		    'network'::text AS category,
		    ('Нода требует внимания: '||n.name)::text AS title,
		    ('Статус '||n.status::text||', роль '||n.role::text)::text AS detail,
		    'node'::text AS resource_type,
		    n.id::text AS resource_id,
		    COALESCE(n.last_heartbeat_at,n.updated_at) AS observed_at
		  FROM nodes n
		  WHERE n.status IN ('degraded','quarantined')

		  UNION ALL

		  SELECT
		    'critical','network',
		    ('Нет heartbeat: '||n.name),
		    'Active-нода не присылала heartbeat более 2 минут',
		    'node',n.id::text,
		    COALESCE(n.last_heartbeat_at,n.updated_at)
		  FROM nodes n
		  WHERE n.status='active'
		    AND (n.last_heartbeat_at IS NULL OR n.last_heartbeat_at<now()-interval '2 minutes')

		  UNION ALL

		  SELECT
		    'warning','build',
		    ('Сборка завершилась ошибкой: '||j.target),
		    COALESCE(j.error_summary,'Build job failed'),
		    'build_job',j.id::text,
		    COALESCE(j.finished_at,j.created_at)
		  FROM build_jobs j
		  WHERE j.status='failed'

		  UNION ALL

		  SELECT
		    'warning','billing',
		    'Ошибка автопродления',
		    COALESCE(a.error_summary,'Recurring payment failed'),
		    'renewal_attempt',a.id::text,
		    a.updated_at
		  FROM subscription_renewal_attempts a
		  WHERE a.status='failed'
		    AND a.updated_at>=now()-interval '7 days'

		  UNION ALL

		  SELECT
		    'critical','billing',
		    'Автопродление отключено после ошибок',
		    ('Три неуспешные попытки; подписка '||s.id::text),
		    'subscription',s.id::text,
		    s.updated_at
		  FROM subscriptions s
		  WHERE s.auto_renew=false AND s.renewal_failures>=3

		  UNION ALL

		  SELECT
		    CASE WHEN i.severity='critical' THEN 'critical' ELSE 'warning' END,
		    'incident',
		    i.title,
		    i.summary,
		    'incident',i.id::text,
		    i.detected_at
		  FROM incidents i
		  WHERE i.status='open'
		)
		SELECT severity,category,title,detail,resource_type,resource_id,observed_at
		FROM issues
		ORDER BY
		  CASE severity WHEN 'critical' THEN 0 ELSE 1 END,
		  observed_at DESC
		LIMIT $1
	`,limit)
	if err!=nil{return nil,fmt.Errorf("operational issues: %w",err)}
	defer rows.Close()
	out:=make([]OperationalIssue,0)
	for rows.Next(){
		var i OperationalIssue
		if err:=rows.Scan(&i.Severity,&i.Category,&i.Title,&i.Detail,&i.ResourceType,&i.ResourceID,&i.ObservedAt);err!=nil{return nil,err}
		out=append(out,i)
	}
	return out,rows.Err()
}
