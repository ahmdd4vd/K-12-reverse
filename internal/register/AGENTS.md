## OVERVIEW

Paket ini pegang orkestrasi inti registrasi, login rescue, alur K12, worker batch, simpan state runtime.

File utama:
- `internal/register/client.go` — client HTTP, TLS profile, proxy, request logging.
- `internal/register/flow.go` — signup, OTP verify, account create, callback handoff.
- `internal/register/flow_login.go` — login mode, zombie rescue, account lanjut.
- `internal/register/k12.go` — invite workspace, switch workspace, extract token.
- `internal/register/batch.go` — engine worker, retry accounting, output write, session save.

Simbol kunci:
- `NewClient`
- `RunRegister`
- `RunLogin`
- `RunK12Flow`
- `RunBatch`
- `registerOne`
- `saveTokensPerBase`

## WHERE TO LOOK

Ubah flow signup atau OTP: cek `flow.go` lalu dampak ke `batch.go`.

Ubah fallback akun existing atau zombie: cek `flow_login.go` dan pemicu switch di `registerOne`.

Ubah proxy, TLS fingerprint, header, trace request: cek `client.go`.

Ubah invite K12, workspace switch, token export: cek `k12.go` dan call site sukses di worker.

Ubah output sukses, retry, graceful stop, `data/session.json`: cek `batch.go`.

## CONVENTIONS

Mode Gmail ambil alias dari `GmailDotPool`, lalu map balik ke base Gmail untuk app password. OTP harus pakai alias exact; jangan normalisasi alamat.

Error berisi `already exists`, `profile`, atau `log-in/password` diperlakukan sinyal pindah ke login mode. Jaga substring logic bila sentuh fallback.

Sukses tulis dua output: teks hasil dan JSON token per base Gmail. Cek side effect sebelum ubah urutan save.

`RunBatch` simpan `data/session.json` tiap detik. Saat `remaining <= 0`, file dibuang. Ctrl+C set remaining ke `0` untuk stop halus, bukan kill keras.

Log boleh jejak status, jangan bocorkan token, cookie, password, app password, atau secret lain.

## ANTI-PATTERNS

Jangan ubah accounting `remaining`, `retries`, atau counter worker tanpa cek perilaku konkurensi. Bug kecil di sini gampang bikin stop macet atau loop salah.

Jangan cetak token ke terminal tambahan atau log debug. Package ini kena service live.

Jangan tambahkan unit test palsu/mocked untuk flow jaringan penuh tanpa izin. Paket ini sekarang hidup di OpenAI/K12/Gmail real, tanpa suite unit.

Jangan bikin duplicate token save di path sukses. Waspada: `registerOne` dan branch sukses sudah sama-sama bisa panggil `saveTokensPerBase`.

Jangan ubah matching OTP jadi longgar. Alias exact cegah worker rebut kode milik alias lain.

## NOTES

Perubahan package ini idealnya diakhiri `gofmt -w internal/register/*.go` pada file tersentuh.

Validasi aman: build targeted atau run CLI manual terbatas. Validasi penuh kena network live dan kredensial nyata.
