# Security Policy

## Supported Versions

This project maintains exceptionally stringent security requirements. The built-in remote control component is highly sensitive — any successful compromise could result in extremely serious consequences.

**We urge you** to report any discovered security vulnerabilities or risks to the maintainers immediately. 

Once a security update is released, it will be automatically delivered to all online instances with auto-update enabled within **12 hours**.

## Reporting a Vulnerability

If you discover a security vulnerability, **please do not open a public GitHub Issue**. Public disclosure could put users at risk.

### How to Report Privately (Recommended)

1. Go to the **Security** tab of this repository.
2. Click **"Report a vulnerability"** (or use the [direct link](https://github.com/mosona-labs/mosona-manager/security/advisories/new)).
3. Fill in the report form with as much detail as possible, including:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)
   - Affected versions

GitHub will create a **private security advisory** visible only to you and the project maintainers. We will acknowledge your report within 48 hours and work with you on a coordinated disclosure.

### Alternative Reporting Channels

- Email: [arsfy@outlook.com](mailto:arsfy@outlook.com)

We kindly ask reporters to follow responsible disclosure principles and give us reasonable time to investigate and patch before public disclosure.

## Encryption Key Backup

Mosona Manager encrypts stored SSH credentials with a local master key. Docker Compose stores it at `cfg/key` inside the persistent `app_data` volume. Manual deployments should set `MOSONA_ENCRYPTION_KEY_PATH` to an absolute path on persistent storage; existing `./cfg/key` installations are detected automatically.

Back up the key together with PostgreSQL and keep the backup access-restricted. Restore the same key before starting against a restored database, with its directory and file set to `0700` and `0600`. If encrypted credentials exist but the key is missing, unreadable, or invalid, startup intentionally fails instead of generating a replacement. There is currently no automatic key rotation; a rotation must re-encrypt all stored credentials as one coordinated operation.

## Agent Key Storage

Run each Mosona Agent under a dedicated operating-system service account. The Agent install directory and passive-agent private key are restricted to `0700` and `0600` on Unix; startup rejects symlinks, non-regular key files, and files owned by another user. Existing installations with the expected owner are upgraded in place to the restricted modes. Back up `private_key.pem` only to access-restricted storage and reinstall the Agent if the key may have been exposed.

## Disclosure Policy

- We aim to fix confirmed vulnerabilities as quickly as possible.
- Once a fix is ready, we will release a security advisory and patch.
- We credit reporters in the advisory (unless you prefer to remain anonymous).

Thank you for helping keep this project secure.
