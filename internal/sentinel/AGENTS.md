## OVERVIEW

Paket ini pegang OpenAI Sentinel challenge dan token generation buat langkah account creation.
Fokus kecil: ambil challenge, rakit payload `p/c/id/flow`, hit proof-of-work ringan, hasilkan `openai-sentinel-token` siap pakai.

## WHERE TO LOOK

- `internal/sentinel/challenge.go` — entry network path. `FetchSentinelChallenge` POST ke Sentinel. `BuildSentinelToken` gabung challenge + generator output.
- `internal/sentinel/generator.go` — `SentinelTokenGenerator`, `NewGenerator`, `GenerateToken`, `GenerateRequirementsToken`.
- `internal/sentinel/fnv.go` — `FNV1a32` hash helper buat difficulty check.
- `internal/register/flow.go` — pemakai utama. `createAccount` set header `openai-sentinel-token`.
- `internal/register/client.go` — sumber HTTP session, TLS impersonation, device/session header behavior. Sinkron wajib.
- `internal/chrome/profiles.go` — referensi fingerprint browser; jaga cocok dengan `User-Agent`, `sec-ch-ua`, platform hints.

## CONVENTIONS

- Ubah shape token hati-hati. JSON hasil `BuildSentinelToken` sekarang `p`, `t`, `c`, `id`, `flow`.
- `deviceID`, `flow`, `ua`, `secChUA`, impersonation path harus tetap selaras dengan caller register flow.
- `NewGenerator` boleh fallback buat `deviceID`/UA kosong, tapi caller normalnya kirim fingerprint nyata dari register client.
- `GenerateRequirementsToken` dipakai saat request awal dan saat proof-of-work tidak diminta.
- `GenerateToken` loop nonce sampai hash prefix lolos difficulty; fallback token ada buat fail-soft, bukan akurasi penuh browser emulation.
- Jika ubah config array generator, cek dampak ke challenge acceptance. Urutan field lebih penting dari nama lokal.

## ANTI-PATTERNS

- Jangan log raw challenge, proof-of-work seed, atau token hasil jadi kecuali debug lokal aman diminta eksplisit.
- Jangan ubah `User-Agent` default, `sdk.js` path, atau screen/time payload acak tanpa cek kecocokan server dan fingerprint layer.
- Jangan putus sinkron dengan `internal/register/client.go` atau `internal/chrome/profiles.go`; mismatch fingerprint mudah bikin create account gagal.
- Jangan ganti `FNV1a32` ke `hash/fnv`; implementasi sekarang pakai avalanche finalizer khusus supaya hasil cocok referensi target.
- Jangan tambah test jaringan otomatis ke Sentinel. Endpoint live, respons berubah, rawan flaky.

## NOTES

- Parameter `impersonate` lewat sampai `BuildSentinelToken`/`FetchSentinelChallenge` walau belum dipakai langsung di file ini; nilai hidup di layer client session.
- Header `Referer` dan `Origin` Sentinel hardcoded. Kalau endpoint/frame version ganti, cek berpasangan dengan payload generator.
- File ini kecil tapi sensitif. Diff pendek lebih aman.
