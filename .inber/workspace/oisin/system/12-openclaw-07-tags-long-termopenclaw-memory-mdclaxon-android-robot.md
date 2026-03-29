- Repo: github.com/kayushkin/claxon-android (private), Kotlin, API 29+
- Voice interface: push-to-talk → SpeechRecognizer → WebSocket to gateway → TTS
- DeviceController for phone control (apps, flashlight, volume, URLs, alarms)
- Will run on spare Galaxy Z Flip 6, potentially mounted on BLE robot chassis
- Cover screen for face/status display
- **APK built** (14MB debug), Android SDK at ~/Android/Sdk, Java 17 pinned via mise
- Ready to sideload via ADB, needs gateway URL+token in settings
- Device is Galaxy Z Flip **5** (not 6), connected USB bus ID 3-4
- WSL ADB can't see USB — use Windows `adb.exe` or `usbipd`

