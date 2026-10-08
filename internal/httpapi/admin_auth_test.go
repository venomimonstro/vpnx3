package httpapi

import (
	"net"
	"testing"
)

func TestAdminSessionContextUserAgent(t *testing.T){
	if !adminSessionContextMatches("user-agent",nil,"UA",nil,"UA"){t.Fatal("same UA must match")}
	if adminSessionContextMatches("user-agent",nil,"UA",nil,"Other"){t.Fatal("different UA must fail")}
}

func TestSameAdminNetworkIPv4(t *testing.T){
	if !sameAdminNetwork(net.ParseIP("203.0.113.10"),net.ParseIP("203.0.113.200")){t.Fatal("same /24 must match")}
	if sameAdminNetwork(net.ParseIP("203.0.113.10"),net.ParseIP("203.0.114.10")){t.Fatal("different /24 must fail")}
}

func TestSameAdminNetworkIPv6(t *testing.T){
	if !sameAdminNetwork(net.ParseIP("2001:db8:1:2::1"),net.ParseIP("2001:db8:1:2::ffff")){t.Fatal("same /64 must match")}
	if sameAdminNetwork(net.ParseIP("2001:db8:1:2::1"),net.ParseIP("2001:db8:1:3::1")){t.Fatal("different /64 must fail")}
}
