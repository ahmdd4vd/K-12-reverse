package util

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatProxy parses various proxy string formats and normalizes them into protocol://username:password@host:port
// Supported formats:
// 1. Agreement://Host IP:Port:Username:Password (e.g. socks5://192.168.1.1:8080:user:pass)
// 2. Host IP:Port:Username:Password
// 3. Username:Password@Host IP:Port
// 4. Username:Password:Host IP:Port
// 5. Host IP:Port@Username:Password
func FormatProxy(rawProxy string) string {
	rawProxy = strings.TrimSpace(rawProxy)
	if rawProxy == "" {
		return ""
	}

	protocol := "http"
	parts := strings.SplitN(rawProxy, "://", 2)
	if len(parts) == 2 {
		protocol = parts[0]
		rawProxy = parts[1]
	}

	var user, pass, host, port string

	// Helper to check if a string looks like a port
	isPort := func(s string) bool {
		p, err := strconv.Atoi(s)
		return err == nil && p > 0 && p <= 65535
	}

	if strings.Contains(rawProxy, "@") {
		atParts := strings.SplitN(rawProxy, "@", 2)
		part1 := atParts[0]
		part2 := atParts[1]

		p1Parts := strings.SplitN(part1, ":", 2)
		if len(p1Parts) == 2 && isPort(p1Parts[1]) && (strings.Contains(p1Parts[0], ".") || p1Parts[0] == "localhost") {
			// Host:Port@Username:Password
			host = p1Parts[0]
			port = p1Parts[1]
			p2Parts := strings.SplitN(part2, ":", 2)
			if len(p2Parts) == 2 {
				user = p2Parts[0]
				pass = p2Parts[1]
			} else {
				user = part2
			}
		} else {
			// Username:Password@Host:Port (Standard)
			p1Parts := strings.SplitN(part1, ":", 2)
			if len(p1Parts) == 2 {
				user = p1Parts[0]
				pass = p1Parts[1]
			} else {
				user = part1
			}
			p2Parts := strings.SplitN(part2, ":", 2)
			if len(p2Parts) == 2 {
				host = p2Parts[0]
				port = p2Parts[1]
			} else {
				host = part2
			}
		}
	} else {
		colonParts := strings.Split(rawProxy, ":")
		if len(colonParts) == 4 {
			if isPort(colonParts[1]) && (strings.Contains(colonParts[0], ".") || colonParts[0] == "localhost") {
				// Host:Port:Username:Password
				host = colonParts[0]
				port = colonParts[1]
				user = colonParts[2]
				pass = colonParts[3]
			} else if isPort(colonParts[3]) && (strings.Contains(colonParts[2], ".") || colonParts[2] == "localhost") {
				// Username:Password:Host:Port
				user = colonParts[0]
				pass = colonParts[1]
				host = colonParts[2]
				port = colonParts[3]
			} else {
				// Fallback to Host:Port:Username:Password
				host = colonParts[0]
				port = colonParts[1]
				user = colonParts[2]
				pass = colonParts[3]
			}
		} else if len(colonParts) == 2 {
			// Host:Port
			host = colonParts[0]
			port = colonParts[1]
		} else {
			if !strings.Contains(rawProxy, "://") {
				return fmt.Sprintf("%s://%s", protocol, rawProxy)
			}
			return rawProxy
		}
	}

	if user != "" || pass != "" {
		return fmt.Sprintf("%s://%s:%s@%s:%s", protocol, user, pass, host, port)
	}
	if host != "" && port != "" {
		return fmt.Sprintf("%s://%s:%s", protocol, host, port)
	}

	return fmt.Sprintf("%s://%s", protocol, rawProxy)
}
