package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func LocalMetadata() map[string]any {
	out := map[string]any{
		"goos":runtime.GOOS,
		"goarch":runtime.GOARCH,
		"cpu_count":runtime.NumCPU(),
	}
	if raw,err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields)>=3 {
			out["load_1"]=fields[0]
			out["load_5"]=fields[1]
			out["load_15"]=fields[2]
		}
	}
	if f,err := os.Open("/proc/meminfo"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line,"MemTotal:") || strings.HasPrefix(line,"MemAvailable:") {
				parts := strings.Fields(line)
				if len(parts)>=2 {
					v,_ := strconv.ParseInt(parts[1],10,64)
					if strings.HasPrefix(line,"MemTotal:") { out["memory_total_kb"]=v }
					if strings.HasPrefix(line,"MemAvailable:") { out["memory_available_kb"]=v }
				}
			}
		}
	}
	if updates:=RuntimeUpdateMetadata("/run/vpnx3-updater");len(updates)>0{
		out["runtime_updates"]=updates
	}
	return out
}

type RuntimeUpdateStatus struct {
	Target string `json:"target"`
	Version string `json:"version,omitempty"`
	LastCheckAt time.Time `json:"last_check_at"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	Updated bool `json:"updated"`
	Healthy bool `json:"healthy"`
	Error string `json:"error,omitempty"`
}

func RuntimeUpdateMetadata(dir string)[]RuntimeUpdateStatus{
	entries,err:=os.ReadDir(dir);if err!=nil{return nil}
	out:=make([]RuntimeUpdateStatus,0,len(entries))
	for _,entry:=range entries{
		if entry.IsDir()||filepath.Ext(entry.Name())!=".json"{continue}
		raw,err:=os.ReadFile(filepath.Join(dir,entry.Name()));if err!=nil||len(raw)>64<<10{continue}
		var status RuntimeUpdateStatus
		if json.Unmarshal(raw,&status)!=nil||strings.TrimSpace(status.Target)==""{continue}
		if len(status.Error)>300{status.Error=status.Error[:300]}
		out=append(out,status)
	}
	return out
}
