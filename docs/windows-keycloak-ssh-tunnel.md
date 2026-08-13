# Access Keycloak from Windows through an SSH tunnel

The SSH command forwards local Windows ports to services reachable through the remote host:

- `localhost:443` → `172.19.255.200:443` (Keycloak/HTTPS)
- `localhost:18080` → `127.0.0.1:18080` on the SSH server

The browser must also resolve `auth.ai-platform.local` to `127.0.0.1`.

## 1. Add the hostname to the Windows hosts file

Open **Notepad as Administrator**, then open:

```text
C:\Windows\System32\drivers\etc\hosts
```

Add the entry **after** the Tailscale-managed section:

```text
# TailscaleHostsSectionEnd

# AI Platform through SSH tunnel
127.0.0.1 auth.ai-platform.local
```

Do not put the entry between `# TailscaleHostsSectionStart` and `# TailscaleHostsSectionEnd`. Tailscale rewrites that section and may remove custom entries.

Save the file, then run:

```powershell
ipconfig /flushdns
ping auth.ai-platform.local
Resolve-DnsName auth.ai-platform.local
```

The hostname should resolve to:

```text
127.0.0.1
```

## 2. Start and keep the SSH tunnel running

Run this in PowerShell:

```powershell
ssh -N `
  -L 443:172.19.255.200:443 `
  -L 18080:127.0.0.1:18080 `
  ansible@192.168.0.58
```

The same command on one line is:

```powershell
ssh -N -L 443:172.19.255.200:443 -L 18080:127.0.0.1:18080 ansible@192.168.0.58
```

Keep this PowerShell window open. The `-N` option tells SSH not to start a remote shell, so the terminal appears idle after authentication. This is expected: it is forwarding traffic in the background of that window.

## 3. Test Keycloak from a second PowerShell window

```powershell
curl.exe -k -I https://auth.ai-platform.local
Test-NetConnection auth.ai-platform.local -Port 443
```

Expected results:

- `curl.exe` returns an HTTP response rather than a DNS or connection error.
- `Test-NetConnection` shows `TcpTestSucceeded : True`.

Then open:

```text
https://auth.ai-platform.local
```

The `-k` option is only used by the command-line test to ignore an untrusted development certificate. A browser may still display a certificate warning.

## Quick troubleshooting

- **Hostname not resolving:** verify the entry is outside the Tailscale section and run `ipconfig /flushdns` again.
- **Port 443 test fails:** make sure the SSH tunnel window is still open and contains no SSH error.
- **Permission denied while binding port 443:** reopen PowerShell as Administrator and start the tunnel again.
- **Browser certificate warning:** expected when the certificate is self-signed or not trusted by Windows.
