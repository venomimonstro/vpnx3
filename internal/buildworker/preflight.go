package buildworker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Preflight(targets []string) error {
	if _,err:=exec.LookPath("git");err!=nil{return fmt.Errorf("git is required")}
	for _,target:=range targets {
		switch target {
		case "android_apk","android_aab":
			if _,err:=exec.LookPath("gradle");err!=nil{return fmt.Errorf("gradle is required for Android builds")}
			sdk:=strings.TrimSpace(os.Getenv("ANDROID_SDK_ROOT"))
			if sdk==""{sdk=strings.TrimSpace(os.Getenv("ANDROID_HOME"))}
			if sdk=="" { return fmt.Errorf("ANDROID_SDK_ROOT or ANDROID_HOME is required") }
			if stat,err:=os.Stat(filepath.Clean(sdk));err!=nil||!stat.IsDir(){return fmt.Errorf("Android SDK directory is unavailable")}
			if _,err:=os.Stat(filepath.Join(sdk,"platforms","android-37","android.jar"));err!=nil{
				return fmt.Errorf("Android compileSdk 37 platform is unavailable")
			}
			required:=[]string{
				"VPNX3_ANDROID_KEYSTORE_PATH","VPNX3_ANDROID_KEYSTORE_PASSWORD",
				"VPNX3_ANDROID_KEY_ALIAS","VPNX3_ANDROID_KEY_PASSWORD",
				"VPNX3_CLIENT_CONTROL_URL","VPNX3_CONFIG_PUBLIC_KEY",
			}
			for _,key:=range required {
				if strings.TrimSpace(os.Getenv(key))=="" { return fmt.Errorf("%s is required for Android release builds",key) }
			}
			if _,err:=os.Stat(os.Getenv("VPNX3_ANDROID_KEYSTORE_PATH"));err!=nil{return fmt.Errorf("Android keystore is unavailable")}
		case "chrome_zip","firefox_zip":
			if _,err:=exec.LookPath("zip");err!=nil{return fmt.Errorf("zip is required for browser extension builds")}
			if _,err:=exec.LookPath("python3");err!=nil{return fmt.Errorf("python3 is required for browser extension validation")}
			if !strings.HasPrefix(strings.TrimSpace(os.Getenv("VPNX3_CLIENT_CONTROL_URL")),"https://") {
				return fmt.Errorf("VPNX3_CLIENT_CONTROL_URL=https://... is required for browser extension builds")
			}
			if strings.TrimSpace(os.Getenv("VPNX3_CONFIG_PUBLIC_KEY"))=="" {
				return fmt.Errorf("VPNX3_CONFIG_PUBLIC_KEY is required for browser extension builds")
			}
		case "ios_ipa":
			if runtime.GOOS!="darwin"{return fmt.Errorf("ios_ipa target requires macOS")}
			for _,tool:=range []string{"xcodebuild","xcodegen","xcrun","go","make","python3","codesign","unzip","plutil"}{
				if _,err:=exec.LookPath(tool);err!=nil{return fmt.Errorf("%s is required for iOS builds",tool)}
			}
			for _,key:=range []string{"VPNX3_IOS_TEAM_ID","VPNX3_IOS_EXPORT_OPTIONS_PLIST","VPNX3_CLIENT_CONTROL_URL","VPNX3_CONFIG_PUBLIC_KEY"}{
				if strings.TrimSpace(os.Getenv(key))==""{return fmt.Errorf("%s is required for iOS builds",key)}
			}
			if _,err:=os.Stat(os.Getenv("VPNX3_IOS_EXPORT_OPTIONS_PLIST"));err!=nil{return fmt.Errorf("iOS ExportOptions.plist is unavailable")}
		case "controlplane_linux_amd64","node_agent_linux_amd64","vpn_worker_linux_amd64","probe_agent_linux_amd64","ingress_proxy_linux_amd64","build_worker_darwin_arm64","config_mirror_linux_amd64","runtime_updater_linux_amd64":
			if _,err:=exec.LookPath("go");err!=nil{return fmt.Errorf("go toolchain is required for server binary builds")}
		default:
			return fmt.Errorf("unsupported build target %q",target)
		}
	}
	return nil
}
