# Rust TC Studio — Public Update Feed

This repository publishes **player-only** Rust TC Studio updates. The app reads `update.json` from the `main` branch and only accepts manifests verified by its embedded update signing public key.

## Release checklist

1. Build and test the next **player** `Rust_TC_Studio.exe`.
2. Create a GitHub Release, tag `vX.Y.Z`, and upload the player EXE as a release asset.
3. On the private owner PC, run `PUBLISH_AN_UPDATE.bat` (keep `OWNER_UPDATE_SIGNING_KEY.txt` private). Set the version, the local path to that EXE, and the full public GitHub release-asset URL.
4. Commit the generated **signed** `update.json` into this repository's root on `main`.
5. In Rust TC Studio, click **Check for Updates**. A newer verified release can be downloaded and installed from within the app.

Update URL: `https://raw.githubusercontent.com/paintballsavage12/Rust-TC-Studio/main/update.json`

**Never commit or publish** `OWNER_PRIVATE_KEY.txt`, `OWNER_UPDATE_SIGNING_KEY.txt`, the private owner package, or player login codes. A public GitHub repository is not a secure login database; six-digit account codes require a separate server with authentication and rate limiting.

Rust TC Studio is an independent third-party tool. Respect game and server rules, and test releases before publishing.
