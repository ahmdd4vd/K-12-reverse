# EMAIL PACKAGE GUIDE

## OVERVIEW
- Paket ini pegang semua sumber email untuk flow registrasi.
- Cakupan: Gmail IMAP OTP, polling link invite K12, pool Gmail dot-trick, generator dot alias, temp email fallback.
- Semua logic di sini dekat trust boundary: inbox live, app password, alamat target, file queue.

## WHERE TO LOOK
- `internal/email/gmail_imap.go` — OTP Gmail via IMAP. Exact `To` match. Unread only. Recent window. Mark message seen saat sukses.
- `internal/email/gmail_invite.go` — ambil link `https://chatgpt.com/k12-invite?...` dari inbox Gmail.
- `internal/email/gmail_pool.go` — pool multi-file `data/list_*.txt`, interleave list, hapus alias terpakai dari file asal.
- `internal/email/dot_generator.go` — bangun variasi dot-trick untuk satu Gmail base.
- `internal/email/generator.go` — temp email dari `generator.email`, blacklist domain, polling OTP web inbox.

## CONVENTIONS
- Simbol inti IMAP: `GmailIMAPConfig`, `GetVerificationCodeViaIMAP`, `fetchOTPFromGmail`, `GetK12InviteLinkViaIMAP`.
- Simbol inti pool/generator: `NewMultiGmailPool`, `GmailDotPool.Next`, `GmailDotPool.MarkConsumed`, `GenerateDotTrick`, `CreateTempEmail`, `GetVerificationCode`.
- OTP Gmail cari email unread dari OpenAI/ChatGPT. Filter akhir tetap 5 menit terakhir walau search awal lebih lebar.
- Match penerima wajib exact ke alias target, termasuk variasi dot. Jangan canonicalize.
- Subject invite K12 harus dipisah dari OTP. Email subject `approved`/`join` wajib diskip saat parse OTP.
- Beberapa angka 6 digit known-bad memang dibuang eksplisit. Jaga daftar itu bila refactor parse.
- Saat OTP/link ketemu, message ditandai `imap.SeenFlag` supaya tidak dipakai ulang.
- Pool rewrite file sumber setelah `MarkConsumed`. Efek samping persisten, bukan cache memori saja.

## ANTI-PATTERNS
- Jangan tambah lock di dalam `GetVerificationCodeViaIMAP`; comment sudah jelas lock internal dibuang untuk hindari deadlock.
- Jangan normalisasi `targetEmail` sebelum compare `Envelope.To`; itu bisa bikin worker ambil OTP worker lain.
- Jangan ubah scan IMAP jadi broad fetch ribuan email. Tetap sempit: unread, recent, bounded fetch.
- Jangan longgarkan filter subject OTP sampai invite/random HTML number ikut kebaca.
- Jangan print `AppPassword`, isi inbox, `data/accounts_*.json`, atau file token saat debug.
- Jangan ubah `MarkConsumed` jadi non-persisten tanpa cek caller; batch flow andal pada shrinking list file.

## NOTES
- `gmail_pool.go` shuffle per-list lalu interleave antar akun; hasilnya round-robin-ish, bukan urutan file murni.
- `dot_generator.go` hapus dot dulu dari username, lalu hasilkan kombinasi `2^(n-1)-1`.
- `generator.go` pakai `blacklist.json` lokal. Domain fallback hardcoded tetap ada bila fetch domain gagal.
- Package ini kena network live. Validasi aman: `gofmt -w internal/email/*.go` atau review code. Test live butuh opt-in.
