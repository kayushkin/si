# openclaw (0.7, tags: long-term,openclaw-memory-md,downloadstack)

*~215 tokens*

- Runs locally in WSL with xvfb on port 4567 (v2.1.1867)
- SSH tunnel to kayushkin.com:4567 so remote downloadstack can use it
- Working sources: MangaPill, Asura Scans, MangaDex
- Cloudflare-blocked: Mangakakalot, MangaFire, ComicK
- Started from modelstack/start.sh alongside TTS server
- Keiyoushi extension repo installed, 7 extensions
- Note: needs chapter detail fetch before page download (populates pageCount)
- Do NOT run on Linode — crashed the 1GB box (uses 270MB+ RAM)
- `initialOpenInBrowserEnabled = false` in server.conf — prevents JCEF crash on startup
- kayushkin-server runs on port 8080 (not 4567) — no port conflict

