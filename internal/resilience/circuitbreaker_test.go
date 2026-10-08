package resilience

import (
	"testing"
	"time"
)

func TestCircuitBreakerOpensAndRecovers(t *testing.T){
	now:=time.Unix(1000,0).UTC()
	cb:=NewCircuitBreaker(2,10*time.Second)
	if !cb.Allow(now){t.Fatal("closed circuit must allow")}
	cb.Failure(now)
	if !cb.Allow(now){t.Fatal("below threshold must still allow")}
	cb.Failure(now)
	if cb.Allow(now.Add(time.Second)){t.Fatal("open circuit must reject before cooldown")}

	s:=cb.Snapshot()
	if s.State!=StateOpen||s.ConsecutiveFailures!=2{t.Fatalf("unexpected snapshot: %+v",s)}

	probeAt:=now.Add(11*time.Second)
	if !cb.Allow(probeAt){t.Fatal("half-open must allow one probe")}
	if cb.Allow(probeAt){t.Fatal("half-open must allow only one in-flight probe")}
	cb.Success()

	s=cb.Snapshot()
	if s.State!=StateClosed||s.ConsecutiveFailures!=0{t.Fatalf("circuit did not reset: %+v",s)}
	if !cb.Allow(probeAt){t.Fatal("closed circuit must allow after success")}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T){
	now:=time.Unix(2000,0).UTC()
	cb:=NewCircuitBreaker(1,5*time.Second)
	cb.Failure(now)
	if !cb.Allow(now.Add(6*time.Second)){t.Fatal("expected half-open probe")}
	cb.Failure(now.Add(6*time.Second))
	if cb.Snapshot().State!=StateOpen{t.Fatal("failed half-open probe must reopen")}
	if cb.Allow(now.Add(7*time.Second)){t.Fatal("reopened circuit must honor new cooldown")}
}
