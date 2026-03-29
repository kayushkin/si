- Repo: github.com/kayushkin/downloadstack — multi-source media downloader
- Sources: Libgen, Standard Ebooks, Gutenberg, Royal Road, Wuxiaworld, NovelFull, iTunes (podcasts), MangaDex, MangaPill, yt-dlp
- Pipeline: downloadstack → bookstack/inbox → library/epub/
- Rate limiting: 2 concurrent downloads, per-source throttle, request dedup
- Retry: exponential backoff (1s→3s→9s), smart error classification
- Direct API fallbacks: MangaDex API + MangaPill scraping (no Suwayomi needed)
- Source chain: Suwayomi → MangaDex direct → MangaPill direct (auto-detects Suwayomi down)
- Retry queue + `/api/queue` endpoint
- Auto-triggers mangastack rescan after manga download

