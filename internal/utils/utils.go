package utils

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"

	"github.com/skip2/go-qrcode"
)

// localIPFn is the function used to resolve the local IP address.
// Overridden in tests to simulate machines with no network interface.
var localIPFn = LocalIP

func GetURLHost(host string) string {
	urlHost := host
	if host == "0.0.0.0" {
		if ip := localIPFn(); ip != "" {
			urlHost = ip
		} else {
			urlHost = "localhost"
		}
	}
	return urlHost
}

func PrintUrlQRAndOpen(host string, port int, noAuth bool, fixedAuthToken string, noBrowser bool, quiet bool) {
	url := fmt.Sprintf("http://%s:%d", host, port)

	if !noAuth {
		url += "?token=" + fixedAuthToken
	}

	if !quiet {
		fmt.Printf("godom running at\n%s\n", url)
		PrintQR(url)
	}

	if !noBrowser {
		OpenBrowser(url)
	}
}

func LocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

func PrintQR(url string) {
	qr, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return
	}
	bitmap := qr.Bitmap()
	n := len(bitmap)
	for y := 0; y < n; y += 2 {
		for x := 0; x < n; x++ {
			top := bitmap[y][x]
			bot := false
			if y+1 < n {
				bot = bitmap[y+1][x]
			}
			switch {
			case top && bot:
				fmt.Print("█")
			case top:
				fmt.Print("▀")
			case bot:
				fmt.Print("▄")
			default:
				fmt.Print(" ")
			}
		}
		fmt.Println()
	}
}

func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
