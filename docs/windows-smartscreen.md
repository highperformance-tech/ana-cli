# Windows SmartScreen

Official release binaries are Authenticode-signed via Azure Trusted Signing
(see the `sign-windows` job in `.github/workflows/release.yml`), so
SmartScreen should not prompt when running an `ana.exe` downloaded from
GitHub Releases.

Binaries built from source (`go install` / `make build`) are **not** signed
and SmartScreen may still block them on first run with "Windows protected
your PC". Click **More info → Run anyway**, or unblock the file before
running:

```powershell
Unblock-File -Path .\ana.exe
```
