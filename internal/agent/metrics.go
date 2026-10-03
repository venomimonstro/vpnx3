package agent

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
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
	return out
}
