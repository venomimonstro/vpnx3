package probe

import (
	"testing"
	"time"
)

func TestRotatingWorkersBoundedAndDeterministic(t *testing.T){
	workers:=make([]manifestNode,12)
	for i:=range workers{
		workers[i].ID=string(rune('a'+i))
	}
	now:=time.Date(2026,10,4,7,0,0,0,time.UTC)
	first:=rotatingWorkers(workers,"probe-a",5,now)
	second:=rotatingWorkers(workers,"probe-a",5,now)
	if len(first)!=5||len(second)!=5{t.Fatalf("expected 5 workers, got %d/%d",len(first),len(second))}
	for i:=range first{
		if first[i].ID!=second[i].ID{t.Fatalf("rotation must be deterministic within same minute")}
	}
	next:=rotatingWorkers(workers,"probe-a",5,now.Add(time.Minute))
	same:=true
	for i:=range first{
		if first[i].ID!=next[i].ID{same=false;break}
	}
	if same{t.Fatalf("rotation should move across minutes")}
}

func TestRotatingWorkersReturnsAllWhenBelowLimit(t *testing.T){
	workers:=[]manifestNode{{ID:"a"},{ID:"b"}}
	got:=rotatingWorkers(workers,"probe",5,time.Now())
	if len(got)!=2||got[0].ID!="a"||got[1].ID!="b"{t.Fatalf("unexpected selection: %#v",got)}
}
