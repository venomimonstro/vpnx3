package ipam

import (
	"crypto/sha256"
	"fmt"
	"net/netip"
	"sync"
)

type Pool struct {
	mu     sync.Mutex
	prefix netip.Prefix
	first  uint32
	last   uint32
	used   map[netip.Addr]string
	byKey  map[string]netip.Addr
}

func New(cidr string) (*Pool,error) {
	prefix,err:=netip.ParsePrefix(cidr)
	if err!=nil { return nil,fmt.Errorf("parse pool: %w",err) }
	if !prefix.Addr().Is4() { return nil,fmt.Errorf("only IPv4 pools are supported in the first transport") }
	bits:=prefix.Bits()
	if bits<16 || bits>29 { return nil,fmt.Errorf("pool prefix must be between /16 and /29") }
	base:=addrUint32(prefix.Masked().Addr())
	size:=uint32(1) << uint32(32-bits)
	first:=base+2
	last:=base+size-2
	return &Pool{prefix:prefix.Masked(),first:first,last:last,used:map[netip.Addr]string{},byKey:map[string]netip.Addr{}},nil
}

func (p *Pool) Acquire(key string) (netip.Addr,error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if addr,ok:=p.byKey[key]; ok { return addr,nil }

	count:=p.last-p.first+1
	sum:=sha256.Sum256([]byte(key))
	start:=uint32(sum[0])<<24|uint32(sum[1])<<16|uint32(sum[2])<<8|uint32(sum[3])
	start=p.first+(start%count)
	for i:=uint32(0);i<count;i++ {
		v:=p.first+((start-p.first+i)%count)
		addr:=uint32Addr(v)
		if _,exists:=p.used[addr]; !exists {
			p.used[addr]=key
			p.byKey[key]=addr
			return addr,nil
		}
	}
	return netip.Addr{},fmt.Errorf("address pool exhausted")
}

func (p *Pool) Reserve(key,address string) (netip.Addr,error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr,err:=netip.ParseAddr(address)
	if err!=nil || !addr.Is4() { return netip.Addr{},fmt.Errorf("invalid reserved IPv4 address") }
	v:=addrUint32(addr)
	if v<p.first || v>p.last || !p.prefix.Contains(addr) {
		return netip.Addr{},fmt.Errorf("reserved address outside pool")
	}
	if existing,ok:=p.byKey[key]; ok {
		if existing==addr { return addr,nil }
		return netip.Addr{},fmt.Errorf("key already owns another address")
	}
	if owner,used:=p.used[addr]; used && owner!=key {
		return netip.Addr{},fmt.Errorf("address already reserved")
	}
	p.used[addr]=key
	p.byKey[key]=addr
	return addr,nil
}

func (p *Pool) Release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	addr,ok:=p.byKey[key]
	if !ok { return }
	delete(p.byKey,key)
	delete(p.used,addr)
}

func addrUint32(a netip.Addr) uint32 {
	b:=a.As4()
	return uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3])
}

func uint32Addr(v uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(v>>24),byte(v>>16),byte(v>>8),byte(v)})
}
