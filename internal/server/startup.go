package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
)

// StartupConfig holds server startup settings — port, host, auth, browser.
// Separated from Config to distinguish runtime state from server configuration.
type StartupConfig struct {
	Port      int
	Host      string
	NoAuth    bool
	Token     string
	NoBrowser bool
	Quiet     bool
}

// resolveToken returns the auth token to use. If NoAuth is set, returns empty.
// If Token is set, uses it. Otherwise generates a random token.
func (sc *StartupConfig) resolveToken() string {
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

// applyStartupConfig resolves the listen address, prints the URL, opens
// the browser, and returns a net.Listener. Does not serve.
func applyStartupConfig(sc StartupConfig, token string) (net.Listener, error) {
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
