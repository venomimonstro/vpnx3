package buildworker

import "testing"

func TestAndroidVersion(t *testing.T){
	cases:=[]struct{
		in string
		name string
		code int
		ok bool
	}{
		{"0.2.0","0.2.0",2000,true},
		{"v1.2.3","1.2.3",1002003,true},
		{"2026.10.3","2026.10.3",2026010003,true},
		{"1.2","1.2",1002000,true},
		{"1.2.3-beta","1.2.3",1002003,true},
		{"","",0,false},
		{"1.1000.0","",0,false},
		{"2101.0.0","",0,false},
		{"1.a.0","",0,false},
	}
	for _,tc:=range cases{
		name,code,err:=androidVersion(tc.in)
		if tc.ok && err!=nil{t.Fatalf("%q unexpected error: %v",tc.in,err)}
		if !tc.ok && err==nil{t.Fatalf("%q expected error",tc.in)}
		if tc.ok && (name!=tc.name||code!=tc.code){
			t.Fatalf("%q got (%q,%d), want (%q,%d)",tc.in,name,code,tc.name,tc.code)
		}
	}
}

func TestNormalizeExtensionVersion(t *testing.T){
	cases:=map[string]string{
		"v1.2.3":"1.2.3",
		"1.2.3-beta":"1.2.3",
		"2026.10.3":"2026.10.3",
	}
	for in,want:=range cases{
		got,err:=normalizeExtensionVersion(in)
		if err!=nil{t.Fatalf("%q: %v",in,err)}
		if got!=want{t.Fatalf("%q got %q want %q",in,got,want)}
	}
	for _,in:=range []string{"","1.2.3.4.5","1.a","70000.1"}{
		if _,err:=normalizeExtensionVersion(in);err==nil{t.Fatalf("%q expected error",in)}
	}
}
