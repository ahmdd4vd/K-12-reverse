package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/verssache/chatgpt-creator/internal/jwtanalyzer"
	"github.com/verssache/chatgpt-creator/internal/manager"
	"github.com/verssache/chatgpt-creator/internal/ui"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		// No args: scan all accounts from data/
		scanAllAccounts()
		return
	}

	token := args[0]
	if strings.HasPrefix(token, "eyJ") {
		analyzeSingle(token)
		return
	}

	// Maybe it's a file path
	data, err := os.ReadFile(token)
	if err == nil {
		t := strings.TrimSpace(string(data))
		if strings.HasPrefix(t, "eyJ") {
			analyzeSingle(t)
			return
		}
	}

	fmt.Printf("Usage: go run ./cmd/jwtanalyzer/ [token or file path]\n")
	fmt.Printf("       go run ./cmd/jwtanalyzer/  (scan all accounts in data/)\n")
}

func scanAllAccounts() {
	ui.ClearScreen()
	ui.PrintBanner()
	fmt.Println("\n" + ui.C("📋 JWT TOKEN ANALYZER — Scan All Accounts", ui.Cyan))
	fmt.Println(strings.Repeat("─", 60))

	entries, err := manager.LoadAllAccounts("data")
	if err != nil || len(entries) == 0 {
		fmt.Println(ui.C("⚠ No accounts found in data/", ui.Yellow))
		return
	}

	valid := 0
	expired := 0
	for _, entry := range entries {
		token := entry.Token
		info, err := jwtanalyzer.Decode(token.AccessToken)
		if err != nil {
			fmt.Printf("❌ %s — decode error: %v\n", token.Email, err)
			continue
		}
		jwtanalyzer.PrintInfo(info)
		if info.IsExpired {
			expired++
		} else {
			valid++
		}
	}

	fmt.Printf("\n📊 Summary: %d valid, %d expired, %d total\n", valid, expired, len(entries))
}

func analyzeSingle(token string) {
	ui.ClearScreen()
	ui.PrintBanner()
	fmt.Println("\n" + ui.C("🔍 JWT TOKEN ANALYZER", ui.Cyan))

	info, err := jwtanalyzer.Decode(token)
	if err != nil {
		fmt.Printf(ui.C("❌ Error: %v\n", ui.Red), err)
		return
	}

	jwtanalyzer.PrintInfo(info)
}
