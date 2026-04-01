package startup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/anupshinde/godom/internal/env"
	qrcode "github.com/skip2/go-qrcode"
)

// Config holds server startup settings — port, host, auth, browser.
type Config struct {
	Port       int
	Host       string
	NoAuth     bool
	Token      string
	NoBrowser  bool
	Quiet     bool
}

// ApplyEnv reads GODOM_* environment variables for fields not set in code.
func (sc *Config) ApplyEnv() {
	if sc.Port == 0 {
		if v, err := strconv.Atoi(os.Getenv("GODOM_PORT")); err == nil && v != 0 {
			sc.Port = v
		}
	}
	if sc.Host == "" {
		if v := os.Getenv("GODOM_HOST"); v != "" {
			sc.Host = v
		}
	}
	if !sc.NoAuth {
		sc.NoAuth = env.Bool("GODOM_NO_AUTH")
	}
	if sc.Token == "" {
		if v := os.Getenv("GODOM_TOKEN"); v != "" {
			sc.Token = v
		}
	}
	if !sc.NoBrowser {
		sc.NoBrowser = env.Bool("GODOM_NO_BROWSER")
	}
	if !sc.Quiet {
		sc.Quiet = env.Bool("GODOM_QUIET")
	}
}

// ResolveToken returns the auth token to use. If NoAuth is set, returns empty.
// If Token is set, uses it. Otherwise generates a random token.
func (sc *Config) ResolveToken() string {
	if sc.NoAuth {
		return ""
	}
	if sc.Token != "" {
		return sc.Token
	}
	return generateToken()
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("godom: failed to generate auth token: %v", err)
	}
	return hex.EncodeToString(b)
}

// Apply resolves the listen address, prints the URL, opens the browser,
// and returns a net.Listener. Does not serve.
func Apply(sc Config, token string) (net.Listener, error) {
	host := sc.Host
	if host == "" {
		host = "localhost"
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, sc.Port))
	if err != nil {
		return nil, fmt.Errorf("godom: failed to listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	urlHost := host
	if host == "0.0.0.0" {
		if ip := localIP(); ip != "" {
			urlHost = ip
		} else {
			urlHost = "localhost"
		}
	}
	url := fmt.Sprintf("http://%s:%d", urlHost, port)
	if !sc.NoAuth {
		url += "?token=" + token
	}
	if !sc.Quiet {
		fmt.Printf("godom running at\n%s\n", url)
		printQR(url)
	}

	if !sc.NoBrowser {
		openBrowser(url)
	}

	return ln, nil
}

func localIP() string {
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

func printQR(url string) {
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

func openBrowser(url string) {
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
