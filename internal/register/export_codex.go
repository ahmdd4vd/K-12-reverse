package register

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/verssache/chatgpt-creator/internal/config"
	"github.com/verssache/chatgpt-creator/internal/ui"
)

type codexJwtPayload struct {
	AuthData struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
		ChatGPTUserID    string `json:"chatgpt_user_id"`
		UserID           string `json:"user_id"`
	} `json:"https://api.openai.com/auth"`
	Exp int64 `json:"exp"`
}

func parseCodexJWT(token string) codexJwtPayload {
	var payload codexJwtPayload
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return payload
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return payload
		}
	}
	json.Unmarshal(payloadBytes, &payload)
	return payload
}

func base64UrlEncode(data []byte) string {
	str := base64.URLEncoding.EncodeToString(data)
	str = strings.ReplaceAll(str, "=", "")
	str = strings.ReplaceAll(str, "+", "-")
	str = strings.ReplaceAll(str, "/", "_")
	return str
}

// ExportToCodex handles the auto-import and bypass verification for Codex
func ExportToCodex(reader *bufio.Reader) {
	fmt.Println(ui.C("\n=== Export & Bypass Codex Verification ===", ui.Cyan))

	// 1. Scan for accounts JSON files
	files, err := filepath.Glob(filepath.Join("data", "accounts_*.json"))
	if err != nil || len(files) == 0 {
		fmt.Println(ui.C("⚠ Tidak ada berkas akun JSON yang ditemukan di folder data/", ui.Red))
		return
	}

	fmt.Println(ui.C("Pilih berkas JSON akun:", ui.Yellow))
	for i, f := range files {
		fmt.Printf(" [%d] %s\n", i+1, filepath.Base(f))
	}

	fmt.Printf("\nNomor berkas: ")
	fileInput, _ := reader.ReadString('\n')
	fileInput = strings.TrimSpace(fileInput)

	fileIdx, err := strconv.Atoi(fileInput)
	if err != nil || fileIdx < 1 || fileIdx > len(files) {
		fmt.Println(ui.C("❌ Pilihan tidak valid. Batal.", ui.Red))
		return
	}
	selectedFile := files[fileIdx-1]

	data, err := os.ReadFile(selectedFile)
	if err != nil {
		fmt.Printf(ui.C("⚠ Gagal membaca berkas %s: %v\n", ui.Red), filepath.Base(selectedFile), err)
		return
	}

	var tokens []*TokenResult
	if err := json.Unmarshal(data, &tokens); err != nil {
		fmt.Printf(ui.C("⚠ Gagal mengurai JSON: %v\n", ui.Red), err)
		return
	}

	// Buat list akun yang valid
	var validTokens []*TokenResult
	for _, t := range tokens {
		if t != nil && t.Email != "" && t.AccessToken != "" {
			validTokens = append(validTokens, t)
		}
	}

	if len(validTokens) == 0 {
		fmt.Println(ui.C("⚠ Tidak ada token valid di dalam berkas ini.", ui.Red))
		return
	}

	fmt.Println(ui.C("\nPilih Token / Workspace yang mau di-inject ke Codex:", ui.Yellow))
	for i, t := range validTokens {
		jwtData := parseCodexJWT(t.AccessToken)
		plan := jwtData.AuthData.ChatGPTPlanType
		if plan == "" {
			plan = "free"
		}
		wkID := jwtData.AuthData.ChatGPTAccountID
		if wkID != "" && len(wkID) > 8 {
			wkID = wkID[:8]
		}
		fmt.Printf(" [%d] %s [%s] - Workspace: %s\n", i+1, t.Email, plan, wkID)
	}

	fmt.Printf("\nNomor token: ")
	tokenInput, _ := reader.ReadString('\n')
	tokenInput = strings.TrimSpace(tokenInput)

	tokenIdx, err := strconv.Atoi(tokenInput)
	if err != nil || tokenIdx < 1 || tokenIdx > len(validTokens) {
		fmt.Println(ui.C("❌ Pilihan tidak valid. Batal.", ui.Red))
		return
	}

	selectedToken := validTokens[tokenIdx-1]
	emailClean := strings.ToLower(strings.TrimSpace(selectedToken.Email))

	// 2. Build Synthetic ID Token
	jwtData := parseCodexJWT(selectedToken.AccessToken)
	
	iat := time.Now().Unix()
	exp := jwtData.Exp
	if exp == 0 {
		exp = iat + (30 * 24 * 3600) // Default 30 days
	}

	headerMap := map[string]interface{}{
		"alg":           "none",
		"typ":           "JWT",
		"cpa_synthetic": true,
	}
	
	payloadMap := map[string]interface{}{
		"iat": iat,
		"exp": exp,
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": jwtData.AuthData.ChatGPTAccountID,
			"chatgpt_plan_type":  jwtData.AuthData.ChatGPTPlanType,
			"chatgpt_user_id":    jwtData.AuthData.ChatGPTUserID,
			"user_id":            jwtData.AuthData.UserID,
		},
		"email": emailClean,
	}

	headerBytes, _ := json.Marshal(headerMap)
	payloadBytes, _ := json.Marshal(payloadMap)

	syntheticIdToken := fmt.Sprintf("%s.%s.synthetic", base64UrlEncode(headerBytes), base64UrlEncode(payloadBytes))

	// Refresh token fallback
	refreshToken := selectedToken.RefreshToken
	if refreshToken == "" || refreshToken == "not available" {
		refreshToken = "placeholder"
	}

	// 3. Build auth.json structure
	authConfig := map[string]interface{}{
		"auth_mode":      "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]interface{}{
			"id_token":      syntheticIdToken,
			"access_token":  selectedToken.AccessToken,
			"refresh_token": refreshToken,
			"account_id":    jwtData.AuthData.ChatGPTAccountID,
		},
		"last_refresh": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}

	authBytes, _ := json.MarshalIndent(authConfig, "", "  ")

	// 4. Auto-Detect OS and Inject
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf(ui.C("❌ Gagal mendapatkan direktori Home: %v\n", ui.Red), err)
		return
	}

	codexDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		fmt.Printf(ui.C("❌ Gagal membuat folder %s: %v\n", ui.Red), codexDir, err)
		return
	}

	authPath := filepath.Join(codexDir, "auth.json")
	if err := os.WriteFile(authPath, authBytes, 0600); err != nil {
		fmt.Printf(ui.C("❌ Gagal menulis file auth.json: %v\n", ui.Red), err)
		return
	}

	fmt.Println(ui.C("\n────────────────────────────────────────────────────", ui.Cyan))
	fmt.Printf(ui.C("✅ Bypassed! auth.json berhasil di-inject ke:\n   %s\n", ui.Green), authPath)
	fmt.Println(ui.C("💡 Silakan Buka/Restart aplikasi Codex Anda sekarang.", ui.Yellow))
}
