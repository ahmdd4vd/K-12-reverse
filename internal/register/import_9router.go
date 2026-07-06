package register

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/verssache/chatgpt-creator/internal/ui"
)

type jwtPayload struct {
	AuthData struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

// parseJWT extracts the ChatGPT Account ID and Plan Type from the AccessToken JWT.
func parseJWT(token string) (string, string) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return "", ""
		}
	}
	var payload jwtPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", ""
	}
	return payload.AuthData.ChatGPTAccountID, payload.AuthData.ChatGPTPlanType
}

// find9RouterDB attempts to automatically locate the 9Router data.sqlite file.
// It checks the global default first, then does a fast scan of common local directories.
func find9RouterDB(homeDir string) string {
	defaultPath := filepath.Join(homeDir, ".9router", "db", "data.sqlite")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath // Found in default global path
	}
	
	// Cek path khusus Windows AppData
	appDataPath := filepath.Join(os.Getenv("APPDATA"), "9router", "db", "data.sqlite")
	if _, err := os.Stat(appDataPath); err == nil {
		return appDataPath
	}
	
	nvmPath := `C:\nvm4w\nodejs\node_modules\9router\app\cli\.build-home\.9router\db\data.sqlite`
	if _, err := os.Stat(nvmPath); err == nil {
		return nvmPath // Found in NVM global path
	}

	fmt.Println(ui.C("🔍 Mencari database 9Router di folder lokal (Desktop/Downloads)...", ui.Yellow))

	var foundPath string
	searchDirs := []string{
		filepath.Join(homeDir, "Desktop"),
		filepath.Join(homeDir, "Downloads"),
	}

	for _, dir := range searchDirs {
		if foundPath != "" {
			break
		}
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if foundPath != "" {
				return filepath.SkipDir
			}
			if info.IsDir() {
				name := info.Name()
				// Skip hidden dirs and massive dependency folders to keep scan under 1 second
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "AppData" || name == "Windows" || name == "vendor" {
					return filepath.SkipDir
				}
			}
			if !info.IsDir() && info.Name() == "data.sqlite" {
				parent := filepath.Base(filepath.Dir(path))
				grandparent := filepath.Base(filepath.Dir(filepath.Dir(path)))
				// Must be inside a folder named 'db', inside a folder containing '9router'
				if parent == "db" && strings.Contains(strings.ToLower(grandparent), "9router") {
					foundPath = path
					return filepath.SkipDir
				}
			}
			return nil
		})
	}
	return foundPath
}

// ImportTo9Router imports all tokens from data/accounts_*.json files into 9Router's local SQLite database.
func ImportTo9Router() {
	fmt.Println(ui.C("\n=== Import Accounts to 9Router ===", ui.Cyan))

	// Verify if sqlite3 is installed
	if _, err := exec.LookPath("sqlite3"); err != nil {
		fmt.Println(ui.C("❌ Error: sqlite3 CLI tidak ditemukan di sistem Anda.", ui.Red))
		fmt.Println("Silakan instal sqlite3 terlebih dahulu (e.g. 'sudo apt install sqlite3').")
		return
	}

	// Resolve database path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf(ui.C("❌ Gagal mendapatkan direktori Home: %v\n", ui.Red), err)
		return
	}
	dbPath := find9RouterDB(homeDir)

	// Verify if the DB file exists
	if dbPath == "" {
		fmt.Println(ui.C("❌ Database 9Router tidak ditemukan di folder global maupun lokal.", ui.Red))
		fmt.Println("Pastikan 9Router sudah terinstal dan dijalankan minimal sekali.")
		
		// Fallback: Minta user input manual path
		fmt.Print(ui.C("Masukkan path lengkap ke file data.sqlite 9Router Anda: ", ui.Yellow))
		var manualPath string
		fmt.Scanln(&manualPath)
		manualPath = strings.TrimSpace(manualPath)
		if manualPath == "" {
			return
		}
		if _, err := os.Stat(manualPath); os.IsNotExist(err) {
			fmt.Println(ui.C("❌ File tetap tidak ditemukan. Batal import.", ui.Red))
			return
		}
		dbPath = manualPath
	}

	fmt.Printf(ui.C("✅ Database 9Router ditemukan di: %s\n", ui.Green), dbPath)

	// Scan for accounts JSON files
	files, err := filepath.Glob(filepath.Join("data", "accounts_*.json"))
	if err != nil || len(files) == 0 {
		fmt.Println(ui.C("⚠ Tidak ada berkas akun JSON yang ditemukan di folder data/", ui.Red))
		return
	}

	fmt.Println(ui.C("Daftar berkas akun yang ditemukan:", ui.Yellow))
	for i, f := range files {
		fmt.Printf(" [%d] %s\n", i+1, filepath.Base(f))
	}

	// For convenience, if only one file is found, import it directly.
	// Otherwise, let user choose, or import all.
	var targetFiles []string
	if len(files) == 1 {
		targetFiles = files
		fmt.Printf("\nHanya menemukan 1 berkas. Memproses %s...\n\n", filepath.Base(files[0]))
	} else {
		fmt.Printf("\nPilih nomor berkas untuk diimpor (atau tekan Enter untuk impor SEMUA): ")
		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(input)
		if input == "" {
			targetFiles = files
			fmt.Println("\nMemproses semua berkas...")
		} else {
			var idx int
			_, err := fmt.Sscanf(input, "%d", &idx)
			if err != nil || idx < 1 || idx > len(files) {
				fmt.Println(ui.C("❌ Pilihan tidak valid. Membatalkan.", ui.Red))
				return
			}
			targetFiles = []string{files[idx-1]}
			fmt.Printf("\nMemproses %s...\n\n", filepath.Base(files[idx-1]))
		}
	}

	totalImported := 0
	totalUpdated := 0

	for _, f := range targetFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf(ui.C("⚠ Gagal membaca berkas %s: %v\n", ui.Red), filepath.Base(f), err)
			continue
		}

		var tokens []*TokenResult
		if err := json.Unmarshal(data, &tokens); err != nil {
			fmt.Printf(ui.C("⚠ Gagal mengurai berkas JSON %s: %v\n", ui.Red), filepath.Base(f), err)
			continue
		}

		for _, t := range tokens {
			if t == nil || t.Email == "" || t.AccessToken == "" {
				continue
			}

			emailClean := strings.ToLower(strings.TrimSpace(t.Email))

			// 1. Get ChatGPT Account ID & Plan Type
			accID, planType := parseJWT(t.AccessToken)
			if accID == "" {
				accID = uuid.New().String() // Fallback if jwt decode failed
			}
			if planType == "" {
				planType = "free"
			}

			sqlSafeEmail := strings.ReplaceAll(emailClean, "'", "''")
			sqlSafeAccID := strings.ReplaceAll(accID, "'", "''")
			
			// 2. Query existing ID for this email + workspace in 9Router DB
			queryGetID := fmt.Sprintf("SELECT id FROM providerConnections WHERE provider = 'codex' AND email = '%s' AND json_extract(data, '$.providerSpecificData.chatgptAccountId') = '%s';", sqlSafeEmail, sqlSafeAccID)
			cmdGet := exec.Command("sqlite3", dbPath, queryGetID)
			outBytes, err := cmdGet.Output()
			existingID := strings.TrimSpace(string(outBytes))

			var id string
			isUpdate := false
			if err == nil && existingID != "" {
				id = existingID
				isUpdate = true
			} else {
				id = uuid.New().String()
			}

			// 3. Build data JSON structure expected by 9Router
			nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			
			type providerSpecificData struct {
				ChatgptAccountID string `json:"chatgptAccountId"`
				ChatgptPlanType  string `json:"chatgptPlanType"`
			}
			type connectionData struct {
				AccessToken          string               `json:"accessToken"`
				RefreshToken         string               `json:"refreshToken"`
				TestStatus           string               `json:"testStatus"`
				IdToken              string               `json:"idToken"`
				LastRefreshAt        string               `json:"lastRefreshAt"`
				ProviderSpecificData providerSpecificData `json:"providerSpecificData"`
			}

			connData := connectionData{
				AccessToken:   t.AccessToken,
				RefreshToken:  t.RefreshToken,
				TestStatus:    "unavailable",
				IdToken:       t.IdToken,
				LastRefreshAt: nowStr,
				ProviderSpecificData: providerSpecificData{
					ChatgptAccountID: accID,
					ChatgptPlanType:  planType,
				},
			}

			connDataBytes, err := json.Marshal(connData)
			if err != nil {
				fmt.Printf(ui.C("⚠ Gagal membuat data JSON untuk %s: %v\n", ui.Red), emailClean, err)
				continue
			}

			sqlSafeData := strings.ReplaceAll(string(connDataBytes), "'", "''")

			displayName := fmt.Sprintf("%s [%s]", emailClean, planType)
			if planType == "k12" && accID != "" {
				displayName = fmt.Sprintf("%s [%s]", emailClean, accID[:8])
			}
			sqlSafeName := strings.ReplaceAll(displayName, "'", "''")

			// 4. Insert or Replace query
			queryInsert := fmt.Sprintf(
				"INSERT OR REPLACE INTO providerConnections (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt) VALUES ('%s', 'codex', 'oauth', '%s', '%s', 1, 1, '%s', '%s', '%s');",
				id, sqlSafeName, sqlSafeEmail, sqlSafeData, nowStr, nowStr,
			)

			cmdInsert := exec.Command("sqlite3", dbPath, queryInsert)
			if err := cmdInsert.Run(); err != nil {
				fmt.Printf(ui.C("✗ Gagal mengimpor %s ke 9Router: %v\n", ui.Red), emailClean, err)
			} else {
				if isUpdate {
					fmt.Printf("✓ %s (%s) [UPDATE]\n", ui.C(emailClean, ui.Cyan), ui.C(planType, ui.Green))
					totalUpdated++
				} else {
					fmt.Printf("✓ %s (%s) [BARU]\n", ui.C(emailClean, ui.Cyan), ui.C(planType, ui.Green))
					totalImported++
				}
			}
		}
	}

	fmt.Println(ui.C("\n────────────────────────────────────────────────────", ui.Cyan))
	if totalImported > 0 || totalUpdated > 0 {
		fmt.Printf(ui.C("🎉 Sukses! %d baru diimpor, %d diperbarui ke database 9Router.\n", ui.Green), totalImported, totalUpdated)
	} else {
		fmt.Println(ui.C("⚠ Tidak ada akun baru yang diimpor.", ui.Yellow))
	}
}
