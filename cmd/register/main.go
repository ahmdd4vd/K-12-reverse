package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/verssache/chatgpt-creator/internal/config"
	"github.com/verssache/chatgpt-creator/internal/email"
	"github.com/verssache/chatgpt-creator/internal/register"
	"github.com/verssache/chatgpt-creator/internal/ui"
	"github.com/verssache/chatgpt-creator/internal/updater"
	"github.com/verssache/chatgpt-creator/internal/util"
)

func main() {
	// Tampilkan header
	ui.ClearScreen()
	ui.PrintBanner()

	// Cek pembaruan otomatis
	updater.CheckForUpdates()
	fmt.Println()

	// Ensure data directory exists
	os.MkdirAll("data", 0755)

	// Load config
	cfg, err := config.Load("config.json")
	if err != nil {
		fmt.Printf("%s\n", ui.C(fmt.Sprintf("Error loading config: %v", err), ui.Red))
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	// Auto-detect fresh config
	if cfg.GmailMode && len(cfg.GmailAccounts) == 0 {
		fmt.Println(ui.C("\n[!] Fresh configuration detected. Running setup wizard...", ui.Yellow))
		runSetupWizard(cfg, reader)
	} else if !cfg.GmailMode && cfg.Proxy == "" && cfg.DefaultDomain == "" {
		fmt.Println(ui.C("\n[!] Fresh configuration detected. Running setup wizard...", ui.Yellow))
		runSetupWizard(cfg, reader)
	}

	// Check for interrupted session
	sessionFile := filepath.Join("data", "session.json")
	if sessionData, err := os.ReadFile(sessionFile); err == nil {
		type SessionData struct {
			TotalAccounts int64 `json:"totalAccounts"`
			MaxWorkers    int   `json:"maxWorkers"`
			Remaining     int64 `json:"remaining"`
		}
		var sess SessionData
		if json.Unmarshal(sessionData, &sess) == nil && sess.Remaining > 0 {
			ui.ClearScreen()
			ui.PrintBanner()
			fmt.Printf("%s\n", ui.C("\n[!] Ditemukan sesi pendaftaran yang terputus!", ui.Yellow))
			fmt.Printf("    Target awal   : %d Akun\n", sess.TotalAccounts)
			fmt.Printf("    Sisa target   : %d Akun\n", sess.Remaining)
			fmt.Printf("    Worker dipakai: %d Worker\n\n", sess.MaxWorkers)
			fmt.Print(ui.C("Lanjutkan sesi ini? (Y/n): ", ui.Yellow))
			optInput, _ := reader.ReadString('\n')
			optInput = strings.TrimSpace(strings.ToLower(optInput))

			if optInput == "" || optInput == "y" {
				// We need to initialize gmailPool and other required things
				var k12WorkspaceIDs []string
				if cfg.EnableK12Invite {
					k12WorkspaceIDs = cfg.K12WorkspaceIDs
				}

				var gmailPool *email.GmailDotPool
				if cfg.GmailMode && len(cfg.GmailAccounts) > 0 {
					var listFiles []string
					for _, acc := range cfg.GmailAccounts {
						listFiles = append(listFiles, acc.ListFile)
					}
					var err error
					gmailPool, err = email.NewMultiGmailPool(listFiles)
					if err != nil {
						fmt.Printf("%s\n", ui.C(fmt.Sprintf("⚠ Gagal memuat Gmail Pool: %v", err), ui.Red))
					}
				}

				batchCfg := &register.BatchConfig{
					TotalAccounts:   int(sess.Remaining),
					OutputFile:      cfg.OutputFile,
					MaxWorkers:      sess.MaxWorkers,
					Proxy:           cfg.Proxy,
					DefaultPassword: cfg.DefaultPassword,
					DefaultDomain:   cfg.DefaultDomain,
					K12WorkspaceIDs: k12WorkspaceIDs,
					GmailMode:       cfg.GmailMode,
					GmailPool:       gmailPool,
					GmailAccounts:   cfg.GmailAccounts,
				}

				os.Remove(sessionFile) // clear session to avoid infinite loop on crash

				fmt.Printf("%s\n", ui.C(fmt.Sprintf("\n🚀 Melanjutkan sisa %d akun dengan %d worker...", sess.Remaining, sess.MaxWorkers), ui.Green))
				register.RunBatch(batchCfg)

				fmt.Println()
				fmt.Print(ui.C("Tekan Enter untuk kembali ke Menu Utama...", ui.Yellow))
				reader.ReadString('\n')
			} else {
				os.Remove(sessionFile)
			}
		}
	}

	for {
		ui.ClearScreen()
		ui.PrintBanner()
		printMainMenu(cfg)
		fmt.Printf(ui.C("💡 Pilih opsi (default: 1): ", ui.Yellow))
		optInput, _ := reader.ReadString('\n')
		optInput = strings.TrimSpace(optInput)
		if optInput == "" {
			optInput = "1"
		}

		switch optInput {
		case "1":
			startRegistration(cfg, reader)
		case "2":
			runSetupWizard(cfg, reader)
		case "3":
			printHelpGuide(reader)
		case "4":
			exportTokensTXT(reader)
		case "5":
			register.ImportTo9Router()
			fmt.Print(ui.C("\nTekan Enter untuk kembali ke Menu Utama...", ui.Yellow))
			reader.ReadString('\n')
		case "6":
			register.ExportToCodex(reader)
			fmt.Print(ui.C("\nTekan Enter untuk kembali ke Menu Utama...", ui.Yellow))
			reader.ReadString('\n')
		case "0":
			fmt.Println(ui.C("Goodbye! 👋", ui.Cyan))
			return
		default:
			fmt.Println(ui.C("⚠ Invalid option.", ui.Red))
			reader.ReadString('\n') // wait a bit so user can see error
		}
	}
}

func countLines(filepath string) int {
	file, err := os.Open(filepath)
	if err != nil {
		return 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}

func countJSONTokens(filepath string) int {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return 0
	}
	var tokens []interface{}
	if err := json.Unmarshal(data, &tokens); err != nil {
		return 0
	}
	return len(tokens)
}

func printMainMenu(cfg *config.Config) {
	fmt.Println(ui.C("\n📊 [ GMAIL ACCOUNTS STATUS ]", ui.Cyan))

	if cfg.GmailMode {
		if len(cfg.GmailAccounts) == 0 {
			fmt.Println(ui.C("  (No Gmail accounts configured)", ui.Red))
		} else {
			for i, acc := range cfg.GmailAccounts {
				username := strings.Split(acc.BaseEmail, "@")[0]
				listFile := filepath.Join("data", fmt.Sprintf("list_%s.txt", username))
				tokenFile := filepath.Join("data", fmt.Sprintf("accounts_%s.json", username))

				listCount := countLines(listFile)
				tokenCount := countJSONTokens(tokenFile)

				fmt.Printf(" %d. 🟢 %s\n", i+1, ui.C(acc.BaseEmail, ui.Green))
				fmt.Printf("    └─ Sisa Antrean : %s 📧\n", ui.C(fmt.Sprintf("%d", listCount), ui.Yellow))
				fmt.Printf("    └─ Akun Sukses  : %s 🔑\n", ui.C(fmt.Sprintf("%d", tokenCount), ui.Green))
			}
		}
	} else {
		fmt.Printf(" - Mode: Standard (Domain: %s)\n", cfg.DefaultDomain)
	}

	fmt.Println(ui.C("\n⚙️ [ GLOBAL SETTINGS ]", ui.Cyan))
	if cfg.EnableK12Invite {
		k12 := "(no ID)"
		if len(cfg.K12WorkspaceIDs) > 0 {
			k12 = cfg.K12WorkspaceIDs[0]
		}
		fmt.Printf(" - K12 Invite : ✅ %s (ID: %s)\n", ui.C("ENABLED", ui.Green), k12)
	} else {
		fmt.Printf(" - K12 Invite : ❌ %s\n", ui.C("DISABLED", ui.Red))
	}

	proxyStr := "❌ (none)"
	if cfg.Proxy != "" {
		proxyStr = "✅ " + cfg.Proxy
	}
	pwStr := "🎲 (random)"
	if cfg.DefaultPassword != "" {
		pwStr = "🔒 " + cfg.DefaultPassword
	}
	fmt.Printf(" - Proxy      : %s\n", proxyStr)
	fmt.Printf(" - Default PW : %s\n", pwStr)
	fmt.Println(ui.C("────────────────────────────────────────────────────", ui.Cyan))
	fmt.Println(ui.C("🚀 [1] Start Registration", ui.Green))
	fmt.Println(ui.C("🔧 [2] Edit Configuration & Gmail Accounts", ui.Yellow))
	fmt.Println(ui.C("📚 [3] Bantuan & Panduan Penggunaan", ui.Cyan))
	fmt.Println(ui.C("💾 [4] Export Semua Token ke TXT", ui.Purple))
	fmt.Println(ui.C("🤖 [5] Import Semua Token ke 9Router", ui.Cyan))
	fmt.Println(ui.C("🔓 [6] Export & Bypass Codex Verif", ui.Green))
	fmt.Println(ui.C("❌ [0] Exit", ui.Red))
	fmt.Println(ui.C("────────────────────────────────────────────────────", ui.Cyan))
}

func printHelpGuide(reader *bufio.Reader) {
	fmt.Println(ui.C("\n╔══════════════════════════════════════════════════╗", ui.Cyan))
	fmt.Println(ui.C("║            📚 PANDUAN PENGGUNAAN BOT             ║", ui.Cyan))
	fmt.Println(ui.C("╚══════════════════════════════════════════════════╝", ui.Cyan))

	fmt.Println(ui.C("\n1. Cara Kerja Dot-Trick (Titik Ajaib)", ui.Yellow))
	fmt.Println("   Bot akan otomatis menambahkan titik di antara huruf username Gmail Anda.")
	fmt.Println("   ChatGPT menganggap itu email yang berbeda, tapi Google tetap mengirim")
	fmt.Println("   semua OTP ke satu inbox utama Anda.")
	fmt.Println("   " + ui.C("PENTING:", ui.Red) + " Akun induk (tanpa titik) TIDAK akan didaftarkan untuk mencegah error.")

	fmt.Println(ui.C("\n2. Fitur Auto-Purge (Pembersih Otomatis)", ui.Yellow))
	fmt.Println("   Setiap kali bot berhasil membuat akun, atau mendeteksi bahwa email")
	fmt.Println("   tersebut ZOMBIE (sudah terdaftar/nyangkut), email tersebut akan dihapus")
	fmt.Println("   dari antrean (Sisa List) agar bot tidak membuang waktu mengulangnya.")

	fmt.Println(ui.C("\n3. Round-Robin (Pembagian Adil)", ui.Yellow))
	fmt.Println("   Jika Anda mendaftarkan lebih dari 1 Gmail Induk, bot akan membagi")
	fmt.Println("   tugas secara bergantian (selang-seling) ke setiap Gmail yang ada.")
	fmt.Println("   Misal W1 ngerjain Gmail A, W2 ngerjain Gmail B secara bersamaan.")

	fmt.Println(ui.C("\n4. App Password Google", ui.Yellow))
	fmt.Println("   Password yang dimasukkan BUKAN password login Gmail Anda, melainkan")
	fmt.Println("   16 digit 'App Password' dari Pengaturan Keamanan Akun Google Anda.")

	fmt.Println(ui.C("\n────────────────────────────────────────────────────", ui.Cyan))
	fmt.Print(ui.C("Tekan Enter untuk kembali ke Menu Utama...", ui.Yellow))
	reader.ReadString('\n')
}

func exportTokensTXT(reader *bufio.Reader) {
	fmt.Println(ui.C("\n=== Export Tokens ===", ui.Cyan))

	files, err := filepath.Glob(filepath.Join("data", "accounts_*.json"))
	if err != nil || len(files) == 0 {
		fmt.Println(ui.C("⚠ Tidak ada file token JSON yang ditemukan di folder data/", ui.Red))
		reader.ReadString('\n')
		return
	}

	outFile, err := os.Create("export_tokens.txt")
	if err != nil {
		fmt.Printf(ui.C("⚠ Gagal membuat file export: %v\n", ui.Red), err)
		reader.ReadString('\n')
		return
	}
	defer outFile.Close()

	totalExported := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var tokens []register.TokenResult
		if json.Unmarshal(data, &tokens) == nil {
			for _, t := range tokens {
				line := fmt.Sprintf("%s|%s|%s\n", t.Email, t.Password, t.AccessToken)
				outFile.WriteString(line)
				totalExported++
			}
		}
	}

	if totalExported > 0 {
		fmt.Printf("%s\n", ui.C(fmt.Sprintf("✅ Berhasil mengekspor %d token ke file 'export_tokens.txt'!", totalExported), ui.Green))
	} else {
		fmt.Println(ui.C("⚠ Tidak ada token yang bisa diekspor.", ui.Yellow))
	}

	fmt.Print(ui.C("Tekan Enter untuk kembali...", ui.Yellow))
	reader.ReadString('\n')
}

func runSetupWizard(cfg *config.Config, reader *bufio.Reader) {
	fmt.Println(ui.C("\n=== Edit Configuration ===", ui.Cyan))

	// Gmail Accounts Config
	fmt.Println(ui.C("\n--- Gmail Accounts Manager ---", ui.Yellow))
	fmt.Printf("Enable Gmail Dot-Trick Mode? (Y/n) [current: %s]: ", ui.C(map[bool]string{true: "Y", false: "n"}[cfg.GmailMode], ui.Cyan))
	gmailInput, _ := reader.ReadString('\n')
	gmailInput = strings.TrimSpace(strings.ToLower(gmailInput))
	if gmailInput == "y" {
		cfg.GmailMode = true
	} else if gmailInput == "n" {
		cfg.GmailMode = false
	}

	if cfg.GmailMode {
		for {
			fmt.Printf("\nConfigured Accounts (%d):\n", len(cfg.GmailAccounts))
			for i, acc := range cfg.GmailAccounts {
				fmt.Printf("%d. %s (List: %s)\n", i+1, acc.BaseEmail, acc.ListFile)
			}

			fmt.Println("\n[A] Add new Gmail account")
			fmt.Println("[R] Remove an account")
			fmt.Println("[D] Done / Continue")
			fmt.Printf("Pilih opsi: ")
			opt, _ := reader.ReadString('\n')
			opt = strings.TrimSpace(strings.ToUpper(opt))

			if opt == "D" || opt == "" {
				break
			} else if opt == "A" {
				fmt.Println(ui.C("\n--- Add New Gmail ---", ui.Cyan))
				fmt.Print(ui.C("Base Gmail Address (e.g. john@gmail.com): ", ui.Yellow))
				baseInput, _ := reader.ReadString('\n')
				baseInput = strings.TrimSpace(baseInput)
				if baseInput == "" {
					continue
				}

				// Normalize base input (remove dots)
				parts := strings.Split(baseInput, "@")
				if len(parts) != 2 || parts[1] != "gmail.com" {
					fmt.Println(ui.C("⚠ Must be a valid @gmail.com address!", ui.Red))
					continue
				}
				normUsername := strings.ReplaceAll(parts[0], ".", "")
				normBase := normUsername + "@gmail.com"

				fmt.Print(ui.C("Gmail App Password (16-chars): ", ui.Yellow))
				appPwInput, _ := reader.ReadString('\n')
				appPwInput = strings.TrimSpace(appPwInput)
				if appPwInput == "" {
					fmt.Println(ui.C("⚠ App Password is required!", ui.Red))
					continue
				}

				listFile := filepath.Join("data", fmt.Sprintf("list_%s.txt", normUsername))

				cfg.GmailAccounts = append(cfg.GmailAccounts, config.GmailAccount{
					BaseEmail:   normBase,
					AppPassword: appPwInput,
					ListFile:    listFile,
				})

				fmt.Printf("Generating dot-trick for %s...\n", normBase)
				total, err := email.GenerateDotTrick(normBase, listFile)
				if err != nil {
					fmt.Printf("%s\n", ui.C(fmt.Sprintf("⚠ Error generating list: %v", err), ui.Red))
				} else {
					fmt.Printf("%s\n", ui.C(fmt.Sprintf("✅ Added %s! Saved %d variations to %s", normBase, total, listFile), ui.Green))
				}

			} else if opt == "R" {
				fmt.Print(ui.C("Masukkan nomor urut akun yang mau dihapus: ", ui.Yellow))
				numStr, _ := reader.ReadString('\n')
				numStr = strings.TrimSpace(numStr)
				num, err := strconv.Atoi(numStr)
				if err == nil && num >= 1 && num <= len(cfg.GmailAccounts) {
					cfg.GmailAccounts = append(cfg.GmailAccounts[:num-1], cfg.GmailAccounts[num:]...)
					fmt.Println(ui.C("✅ Account removed.", ui.Green))
				} else {
					fmt.Println(ui.C("⚠ Invalid number.", ui.Red))
				}
			}
		}
	} else {
		fmt.Print(ui.C("Default domain (current: (random), press Enter to use, or enter new): ", ui.Yellow))
		domainInput, _ := reader.ReadString('\n')
		domainInput = strings.TrimSpace(domainInput)
		if domainInput != "" {
			cfg.DefaultDomain = domainInput
		}
	}

	// 2. K12 Invite Prompts
	fmt.Println(ui.C("\n--- K12 Workspace Setup ---", ui.Yellow))
	k12ModeDefault := "n"
	if cfg.EnableK12Invite {
		k12ModeDefault = "Y"
	}
	fmt.Print(ui.C(fmt.Sprintf("Enable K12 Workspace Invite? (Y/n) [current: %s]: ", k12ModeDefault), ui.Yellow))
	k12ModeInput, _ := reader.ReadString('\n')
	k12ModeInput = strings.TrimSpace(strings.ToLower(k12ModeInput))
	if k12ModeInput == "y" || (k12ModeInput == "" && cfg.EnableK12Invite) {
		cfg.EnableK12Invite = true
	} else if k12ModeInput == "n" {
		cfg.EnableK12Invite = false
	}

	if cfg.EnableK12Invite {
		currentK12 := strings.Join(cfg.K12WorkspaceIDs, ",")
		fmt.Print(ui.C(fmt.Sprintf("K12 Workspace ID (Tekan Enter buat biarin [%s], atau masukin banyak ID dipisah koma): ", currentK12), ui.Yellow))
		k12Input, _ := reader.ReadString('\n')
		k12Input = strings.TrimSpace(k12Input)
		if k12Input != "" {
			parts := strings.Split(k12Input, ",")
			var parsed []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					parsed = append(parsed, p)
				}
			}
			cfg.K12WorkspaceIDs = parsed
		}
	}

	// 3. Misc Setup
	fmt.Println(ui.C("\n--- Misc Setup ---", ui.Yellow))
	for {
		fmt.Print(ui.C(fmt.Sprintf("Proxy (enter to skip) [current: %s]: ", cfg.Proxy), ui.Yellow))
		proxyInput, _ := reader.ReadString('\n')
		proxyInput = strings.TrimSpace(proxyInput)
		if proxyInput == "" {
			break
		}

		fmt.Println(ui.C("🔍 Checking proxy access to ChatGPT...", ui.Cyan))
		status, err := util.CheckProxyAccess(proxyInput)
		if err != nil {
			if status == 403 {
				fmt.Println(ui.C("❌ Proxy rejected: ChatGPT/OpenAI blocked this proxy (HTTP 403)", ui.Red))
				continue
			}
			fmt.Printf(ui.C("❌ Proxy rejected: %v\n", ui.Red), err)
			continue
		}

		cfg.Proxy = proxyInput
		fmt.Printf(ui.C("✅ Proxy OK (HTTP %d)\n", ui.Green), status)
		break
	}

	pwDefault := "(random)"
	if cfg.DefaultPassword != "" {
		pwDefault = cfg.DefaultPassword
	}
	fmt.Print(ui.C(fmt.Sprintf("Default password [current: %s]: ", pwDefault), ui.Yellow))
	pwInput, _ := reader.ReadString('\n')
	pwInput = strings.TrimSpace(pwInput)
	if pwInput != "" {
		cfg.DefaultPassword = pwInput
	}

	fmt.Println(ui.C("\n✅ Configuration saved!", ui.Green))
	if err := cfg.Save("config.json"); err != nil {
		fmt.Printf(ui.C("⚠ Failed to save config.json: %v\n", ui.Red), err)
	}
}

func startRegistration(cfg *config.Config, reader *bufio.Reader) {
	fmt.Println(ui.C("\n=== Start Registration ===", ui.Cyan))

	fmt.Print(ui.C("Total accounts to register: ", ui.Yellow))
	totalInput, _ := reader.ReadString('\n')
	totalInput = strings.TrimSpace(totalInput)

	if totalInput == "" {
		fmt.Println(ui.C("Error: total accounts is required.", ui.Red))
		return
	}
	totalAccounts, err := strconv.Atoi(totalInput)
	if err != nil {
		fmt.Printf("%s\n", ui.C(fmt.Sprintf("Error: invalid number '%s'.", totalInput), ui.Red))
		return
	}

	defaultWorkers := 3
	fmt.Print(ui.C(fmt.Sprintf("Max concurrent workers (default: %d): ", defaultWorkers), ui.Yellow))
	workersInput, _ := reader.ReadString('\n')
	workersInput = strings.TrimSpace(workersInput)

	maxWorkers := defaultWorkers
	if workersInput != "" {
		if val, err := strconv.Atoi(workersInput); err == nil {
			maxWorkers = val
		}
	}

	// Setup Gmail pool if in Gmail mode
	var gmailPool *email.GmailDotPool

	if cfg.GmailMode {
		if len(cfg.GmailAccounts) == 0 {
			fmt.Println(ui.C("⚠ No Gmail accounts configured! Please edit configuration first.", ui.Red))
			return
		}

		var listFiles []string
		for _, acc := range cfg.GmailAccounts {
			listFiles = append(listFiles, acc.ListFile)
			// Ensure list file exists, if not generate it
			if _, err := os.Stat(acc.ListFile); os.IsNotExist(err) {
				fmt.Printf(ui.C("Auto-generating missing list for %s...\n", ui.Cyan), acc.BaseEmail)
				email.GenerateDotTrick(acc.BaseEmail, acc.ListFile)
			}
		}

		gmailPool, err = email.NewMultiGmailPool(listFiles)
		if err != nil {
			fmt.Printf("%s\n", ui.C(fmt.Sprintf("Error loading Gmail lists: %v", err), ui.Red))
			return
		}

		// Cap total accounts to available emails
		if totalAccounts > gmailPool.Remaining() {
			fmt.Printf("%s\n", ui.C(fmt.Sprintf("⚠ Only %d Gmail addresses available across all pools, reducing target from %d", gmailPool.Remaining(), totalAccounts), ui.Yellow))
			totalAccounts = gmailPool.Remaining()
		}
	}

	k12WorkspaceIDs := cfg.K12WorkspaceIDs
	if !cfg.EnableK12Invite {
		k12WorkspaceIDs = nil
	}

	fmt.Println()
	fmt.Println(ui.C("────────────────────────────────────────────────────", ui.Cyan))
	fmt.Printf("%s\n", ui.C(fmt.Sprintf("🚀 Starting run with %d accounts / %d workers...", totalAccounts, maxWorkers), ui.Green))
	fmt.Println(ui.C("────────────────────────────────────────────────────", ui.Cyan))
	fmt.Println()

	fmt.Print(ui.C("[?] Lanjut jalankan? (Y/n): ", ui.Yellow))
	confirmInput, _ := reader.ReadString('\n')
	confirmInput = strings.TrimSpace(strings.ToLower(confirmInput))
	if confirmInput != "" && confirmInput != "y" {
		fmt.Println(ui.C("❌ Dibatalkan.", ui.Red))
		return
	}

	if strings.TrimSpace(cfg.Proxy) != "" {
		fmt.Println(ui.C("🔍 Checking proxy access to ChatGPT...", ui.Cyan))
		status, err := util.CheckProxyAccess(cfg.Proxy)
		if err != nil {
			if status == 403 {
				fmt.Println(ui.C("❌ Proxy rejected: ChatGPT/OpenAI blocked this proxy (HTTP 403)", ui.Red))
				fmt.Println(ui.C("Registration tidak dimulai. Perbaiki proxy di menu [2].", ui.Yellow))
				return
			}
			fmt.Printf(ui.C("❌ Proxy rejected: %v\n", ui.Red), err)
			fmt.Println(ui.C("Registration tidak dimulai. Perbaiki proxy di menu [2].", ui.Yellow))
			return
		}
		fmt.Printf(ui.C("✅ Proxy OK (HTTP %d)\n", ui.Green), status)
	}

	// Build batch config
	batchCfg := &register.BatchConfig{
		TotalAccounts:   totalAccounts,
		OutputFile:      cfg.OutputFile,
		MaxWorkers:      maxWorkers,
		Proxy:           cfg.Proxy,
		DefaultPassword: cfg.DefaultPassword,
		DefaultDomain:   cfg.DefaultDomain,
		K12WorkspaceIDs: k12WorkspaceIDs,
		GmailMode:       cfg.GmailMode,
		GmailPool:       gmailPool,
		GmailAccounts:   cfg.GmailAccounts,
	}

	// Run the batch
	register.RunBatch(batchCfg)

	fmt.Println()
	fmt.Print(ui.C("Tekan Enter untuk kembali ke Menu Utama...", ui.Yellow))
	reader.ReadString('\n')
}
