package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/verssache/chatgpt-creator/internal/ui"
)

const CurrentVersion = "v1.2"
const RepoAPI = "https://api.github.com/repos/ahmdd4vd/K-12-reverse/releases/latest"

type Release struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
}

// CheckForUpdates checks GitHub for a new release and prompts the user to update if one exists.
func CheckForUpdates() {
	fmt.Println(ui.C("🔍 Mengecek pembaruan sistem...", ui.Cyan))
	
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(RepoAPI)
	if err != nil {
		fmt.Println(ui.C("⚠️ Gagal mengecek pembaruan, melewati...", ui.Yellow))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return
	}

	if rel.TagName != "" && rel.TagName != CurrentVersion {
		fmt.Printf(ui.C("\n🎉 UPDATE TERSEDIA: %s (Versi Anda: %s)\n", ui.Green), rel.TagName, CurrentVersion)
		fmt.Printf(ui.C("Deskripsi:\n%s\n\n", ui.White), rel.Body)
		
		fmt.Print(ui.C("Apakah Anda ingin memperbarui sistem secara otomatis sekarang? (Y/n): ", ui.Yellow))
		var choice string
		fmt.Scanln(&choice)
		choice = strings.TrimSpace(strings.ToLower(choice))
		
		if choice == "" || choice == "y" {
			runUpdate()
		} else {
			fmt.Println(ui.C("Pembaruan dilewati. Tekan Enter untuk melanjutkan...", ui.Yellow))
			fmt.Scanln()
		}
	} else {
		fmt.Println(ui.C("✅ Sistem Anda sudah versi terbaru ("+CurrentVersion+").", ui.Green))
		time.Sleep(1 * time.Second) // Memberi waktu user untuk membaca pesan sukses
	}
}

func runUpdate() {
	fmt.Println(ui.C("⬇️ Mengunduh pembaruan (git pull)...", ui.Cyan))
	cmd := exec.Command("git", "pull", "origin", "main")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()
	if err != nil {
		fmt.Println(ui.C("❌ Gagal memperbarui otomatis. Silakan jalankan 'git pull origin main' secara manual.", ui.Red))
	} else {
		fmt.Println(ui.C("✅ Pembaruan berhasil! Silakan jalankan ulang program ini.", ui.Green))
		os.Exit(0)
	}
}
