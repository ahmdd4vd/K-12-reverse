# K12-Reverse ChatGPT Creator

> Otomatisasi Registrasi Akun ChatGPT Skala Besar dengan Fitur K12 Invite dan Multi-Gmail IMAP.

K12-Reverse adalah *tool* berbasis Go (Golang) untuk melakukan registrasi akun ChatGPT secara massal. Dibuat dengan antarmuka CLI yang interaktif, *tool* ini memanfaatkan teknik Dot-Trick pada Gmail dan integrasi IMAP untuk mengekstrak OTP secara otomatis tanpa intervensi manual.

---

## 🚀 Fitur Utama

- **Multi-Gmail Dot-Trick**: Menghasilkan ribuan variasi email unik dari satu akun Gmail dasar tanpa memicu sistem anti-spam.
- **IMAP Auto-Read V2**: Membaca kotak masuk Gmail secara *headless* via protokol IMAP. Fitur terbaru ini sudah disempurnakan (Unread Filter) untuk mencegah kesalahan pembacaan OTP yang kedaluwarsa.
- **Auto Login & Zombie Rescue (New in v1.1!)**: Otomatis mendeteksi akun yang sudah terdaftar tetapi menggantung (Zombie). Sistem akan secara otomatis *switch* dari alur Pendaftaran (Sign-up) menjadi alur Masuk (Login) baik menggunakan metode OTP maupun Password Native, tanpa putus!
- **K12 Auto-Invite**: Menggabungkan akun baru (atau akun Zombie yang berhasil di-rescue) ke dalam *workspace* Edukasi (K12) secara instan, lengkap dengan ekstraksi *Token Session*.
- **Multi-Threading / Workers**: Mendukung registrasi konkurensi (berjalan bersamaan) untuk kecepatan maksimal.
- **Smart Proxy Support**: Mendukung SOCKS5 / HTTP Proxy dengan *auth* URL (contoh: `socks5://user:pass@host:port`).
- **Auto-Resume**: Melanjutkan registrasi yang tertunda akibat kegagalan proxy atau terputusnya koneksi, tepat di titik berhentinya.

---

## 🛠️ Persyaratan Sistem

Sebelum menjalankan program ini, pastikan mesin atau server Anda telah terinstal:

- **Go (Golang)**: Versi 1.20 atau yang lebih baru.
- **Koneksi Internet Stabil**: Disarankan menggunakan Proxy berkualitas (Residential/Static) untuk menghindari limitasi *rate-limit* dari Cloudflare/OpenAI.
- **Akun Gmail**: Akun Gmail utama (*base email*) beserta **App Password**-nya (Sandi Aplikasi).

### Cara Mendapatkan App Password Gmail
Demi keamanan, Anda tidak bisa menggunakan kata sandi asli Gmail. Anda harus membuat Sandi Aplikasi (App Password):
1. Aktifkan **Verifikasi 2 Langkah (2FA)** di akun Google Anda.
2. Masuk ke setelan **Keamanan** akun Google.
3. Cari menu **Sandi Aplikasi** (App Passwords).
4. Buat sandi baru (Pilih "Lainnya", beri nama misalnya "K12-Bot").
5. Salin 16 digit huruf yang muncul (tanpa spasi). Ini adalah kredensial yang akan digunakan dalam *tool*.

---

## 📦 Instalasi & Penggunaan

1. **Kloning Repositori**
   ```bash
   git clone https://github.com/ahmadd4vd/k12-reverse.git
   cd k12-reverse
   ```

2. **Jalankan Program**
   Anda tidak perlu mengatur konfigurasi manual. Program memiliki asisten pengaturan (*wizard*) interaktif:
   ```bash
   go run cmd/register/main.go
   ```

3. **Konfigurasi via CLI**
   Pilih opsi **[2] Edit Configuration & Gmail Accounts**. Anda akan dipandu untuk:
   - Memasukkan *Base Email* (contoh: `nama.email@gmail.com`).
   - Memasukkan *App Password* yang baru saja Anda buat.
   - Mengatur URL Proxy (opsional).
   - *Tool* akan otomatis menghasilkan variasi Dot-Trick dan menyimpannya di direktori `data/`.

4. **Mulai Registrasi**
   Pilih opsi **[1] Start Registration** dari menu utama. Tentukan jumlah *worker* (konkurensi) yang diinginkan, dan program akan berjalan sepenuhnya otomatis.

---

## 🔥 Apa yang baru di v1.2?

Versi 1.2 berfokus pada perbaikan ekstrem di sektor konkurensi (Multi-Threading) saat mengambil OTP dari IMAP:

- **Strict Target IMAP Matching (Concurrency Bypass)**: Pada versi sebelumnya, sistem normalisasi email pada filter inbox IMAP membuat *worker* yang berjalan bersamaan berisiko saling berebut OTP (satu *worker* mencuri OTP milik *worker* lain karena struktur email induknya sama). Versi 1.2 menerapkan pencocokan *To Header* 100% *Exact Match* (termasuk validasi letak titik).
- **Lock-Free Concurrency**: Berkat pencocokan OTP yang super presisi, antrean (Mutex Locking) saat proses permintaan OTP ke OpenAI telah **dihapus sepenuhnya**. Kini puluhan *worker* bisa berjalan secara paralel di detik yang sama, meminta OTP dan membacanya di *inbox* yang sama tanpa *conflict* atau *overlap*. Kecepatan registrasi naik drastis!

---

## ⚡ Pembaruan Versi Sebelumnya (v1.1)

Versi 1.1 memperkenalkan mekanisme pemulihan cerdas:

- **Zombie Rescue Mechanism**: Akun yang menggantung (proses tidak selesai) otomatis dialihkan ke alur Login untuk diselamatkan.
- **Adaptive Authentication (Native Password & OTP)**: Deteksi otomatis metode *Passwordless* (OTP) atau *Native Password* saat login.
- **IMAP Read Status Verification**: Filter khusus untuk hanya membaca email OTP yang belum terbaca (*Unread*), menghindari duplikasi pembacaan.
- **Bypass Halaman "About You"**: Injeksi khusus untuk memintas halaman blokir `/about-you` saat proses Login.

---

## 🤝 Kontribusi

Kontribusi selalu terbuka! Jika Anda memiliki perbaikan kode, optimasi *bypass*, atau fitur baru:
1. *Fork* repositori ini.
2. Buat *branch* fitur Anda (`git checkout -b fitur/NamaFitur`).
3. Lakukan *commit* perubahan (`git commit -m "Menambahkan fitur X"`).
4. *Push* ke *branch* (`git push origin fitur/NamaFitur`).
5. Buka **Pull Request**.

Pastikan kode Anda rapi dan mematuhi konvensi bahasa Go (`gofmt`).

---

## 📄 Lisensi

Didistribusikan di bawah Lisensi MIT. Lihat file `LICENSE` untuk informasi selengkapnya.

> Dibuat oleh **Ahmadd4vd**
