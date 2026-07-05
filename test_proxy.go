package main

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatProxy parses various proxy string formats and normalizes them into protocol://username:password@host:port
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
		// Formats: Username:Password@Host:Port OR Host:Port@Username:Password
		// Standard format is Username:Password@Host:Port
		atParts := strings.SplitN(rawProxy, "@", 2)
		part1 := atParts[0]
		part2 := atParts[1]
		
		// If part1 looks like host:port (and not user:pass)
		// Usually host has a dot, but let's just check if part1 has a valid port suffix
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
		// Formats without @, delimited by colons
		colonParts := strings.Split(rawProxy, ":")
		if len(colonParts) == 4 {
			// Either Host:Port:Username:Password OR Username:Password:Host:Port
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
			// Unknown format, maybe just a hostname or standard URL that bypassed previous checks?
			// Just return the original if we can't parse it well.
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

func main() {
	tests := []string{
		"socks5://192.168.1.1:8080:user1:pass1",
		"192.168.1.2:8080:user2:pass2",
		"user3:pass3@192.168.1.3:8080",
		"user4:pass4:192.168.1.4:8080",
		"192.168.1.5:8080@user5:pass5",
		"http://user6:pass6@192.168.1.6:8080",
		"192.168.1.7:8080",
	}

	for _, t := range tests {
		fmt.Printf("IN:  %s\nOUT: %s\n\n", t, FormatProxy(t))
	}
}
