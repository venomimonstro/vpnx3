package resilience

import (
	"errors"
	"sync"
	"time"
)

var ErrDependencyUnavailable=errors.New("dependency unavailable")

type CircuitState string

const(
	StateClosed CircuitState="closed"
	StateOpen CircuitState="open"
	StateHalfOpen CircuitState="half_open"
)

type Snapshot struct{
	State CircuitState `json:"state"`
	ConsecutiveFailures int `json:"consecutive_failures"`
	Threshold int `json:"threshold"`
	OpenedAt *time.Time `json:"opened_at,omitempty"`
	CooldownSeconds int64 `json:"cooldown_seconds"`
}

type CircuitBreaker struct{
	mu sync.Mutex
	threshold int
	cooldown time.Duration
	failures int
	state CircuitState
	openedAt time.Time
	probeInFlight bool
}

func NewCircuitBreaker(threshold int,cooldown time.Duration)*CircuitBreaker{
	if threshold<1{threshold=1}
	if cooldown<time.Second{cooldown=time.Second}
	return &CircuitBreaker{threshold:threshold,cooldown:cooldown,state:StateClosed}
}

func (c *CircuitBreaker) Allow(now time.Time)bool{
	c.mu.Lock();defer c.mu.Unlock()
	switch c.state{
	case StateOpen:
		if now.Sub(c.openedAt)<c.cooldown{return false}
		c.state=StateHalfOpen
		if c.probeInFlight{return false}
		c.probeInFlight=true
		return true
	case StateHalfOpen:
		if c.probeInFlight{return false}
		c.probeInFlight=true
		return true
	default:
		return true
	}
}

func (c *CircuitBreaker) Success(){
	c.mu.Lock();defer c.mu.Unlock()
	c.failures=0
	c.state=StateClosed
	c.openedAt=time.Time{}
	c.probeInFlight=false
}

func (c *CircuitBreaker) Failure(now time.Time){
	c.mu.Lock();defer c.mu.Unlock()
	c.failures++
	c.probeInFlight=false
	if c.state==StateHalfOpen||c.failures>=c.threshold{
		c.state=StateOpen
		c.openedAt=now
	}
}

func (c *CircuitBreaker) Snapshot()Snapshot{
	c.mu.Lock();defer c.mu.Unlock()
	var opened *time.Time
	if !c.openedAt.IsZero(){v:=c.openedAt;opened=&v}
	return Snapshot{
		State:c.state,ConsecutiveFailures:c.failures,Threshold:c.threshold,
		OpenedAt:opened,CooldownSeconds:int64(c.cooldown/time.Second),
	}
}
