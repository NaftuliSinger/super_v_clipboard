# super_v_clipboard

A small clipboard history for Linux, like Windows' Super+V. It remembers the last 50 things you copied. Press your shortcut, pick one, and it gets pasted into the window you were typing in.

## Platform support

| Works | Doesn't work |
|---|---|
| Ubuntu / GNOME on Wayland (tested) | macOS, Windows (Linux only) |
| GNOME on X11 | Desktops without XWayland |
| Other Linux desktops with XWayland (should work, untested) | Images and files (text only) |
| 64-bit Linux (x86-64, ARM64) | Auto-paste on 32-bit Linux |

Known limits:
- **Terminals**: auto-paste sends Ctrl+V, but most terminals paste with Ctrl+Shift+V. The text is still copied, so press Ctrl+Shift+V yourself.
- **Non-QWERTY layouts** (Dvorak, AZERTY…): Ctrl+V is sent as a physical key, so auto-paste may press the wrong key.
- **Tray icon**: GNOME needs the AppIndicator extension. Ubuntu has it already.
- **Large copies**: text over 1 MB isn't saved to the history (it still pastes normally).
- **Privacy**: history is saved as plain text in `~/.local/share/super_v_clipboard/history.json`, and that includes any passwords you copy.

## How it works

- A background daemon watches the clipboard and saves new text.
- `super_v_clipboard show` tells the daemon (over a Unix socket) to open its window.
- Wayland doesn't let apps type into other windows, so for auto-paste the daemon makes a tiny virtual keyboard through `/dev/uinput` that can only press Ctrl+V.

Written in Go. The window and tray icon use [Fyne](https://fyne.io), and clipboard access uses [golang.design/x/clipboard](https://github.com/golang-design/clipboard).

## Install

### 1. Get it

Pick **one** option.

**Option A: download a ready-made build (easiest, no Go needed).** For 64-bit (x86-64) Ubuntu 24.04 or newer, or a similar distro.
```bash
wget https://github.com/naftulisinger/super_v_clipboard/releases/latest/download/super_v_clipboard-linux-amd64.tar.gz
tar xzf super_v_clipboard-linux-amd64.tar.gz
cd super_v_clipboard
```
You can also download it by hand from the [Releases page](https://github.com/naftulisinger/super_v_clipboard/releases).

**Option B: build from source.** Use this on older distros, other CPUs, or if you'd rather build it yourself.
```bash
sudo apt install git golang gcc libgl1-mesa-dev xorg-dev
git clone https://github.com/naftulisinger/super_v_clipboard.git
cd super_v_clipboard
go build -o super_v_clipboard .
```
You need **Go 1.23 or newer** (check with `go version`). Ubuntu 24.04's `golang` is too old, so on 24.04 install Go with `sudo snap install go --classic` instead.

### 2. Set it up

**Run these steps from the `super_v_clipboard` folder** you just went into.

1. Install the program:
   ```bash
   mkdir -p ~/.local/bin
   install -m755 super_v_clipboard ~/.local/bin/
   ```

2. Allow auto-paste (one time). This lets the app press Ctrl+V for you:
   ```bash
   sudo cp 60-super-v-clipboard-uinput.rules /etc/udev/rules.d/
   sudo udevadm control --reload
   sudo udevadm trigger --name-match=uinput
   ```
   If you skip this step, picking an entry only copies it, and you paste it yourself.

3. Start it automatically at login:
   ```bash
   mkdir -p ~/.config/systemd/user
   cp super_v_clipboard.service ~/.config/systemd/user/
   systemctl --user daemon-reload
   systemctl --user enable --now super_v_clipboard
   ```

4. Add a keyboard shortcut. Print the exact command to use:
   ```bash
   echo "$HOME/.local/bin/super_v_clipboard show"
   ```
   Then open **Settings → Keyboard → Custom Shortcuts**, add a shortcut, paste that output as the command, and give it a key such as Super+V.

## Usage

- Press your shortcut (or use the tray icon) to open the history.
- **↑ / ↓** to move, **Enter** or click to paste, **Esc** to close.
- **Clear history** deletes everything.

## Troubleshooting

```bash
journalctl --user -u super_v_clipboard -n 20
```
- `auto-paste disabled ... permission denied`: redo step 2.2, then log out and back in.
- `super_v_clipboard daemon is not running`: run `systemctl --user start super_v_clipboard`.
- `version 'GLIBC_2.xx' not found` (downloaded build): your distro is too old for it. Use Option B to build from source.

## Updating

Get the new version the same way as in step 1 (download it again, or `git pull` and `go build -o super_v_clipboard .`). Then, from the `super_v_clipboard` folder:
```bash
install -m755 super_v_clipboard ~/.local/bin/super_v_clipboard.new && mv ~/.local/bin/super_v_clipboard.new ~/.local/bin/super_v_clipboard
systemctl --user restart super_v_clipboard
```

## Uninstall

These commands work from any folder:
```bash
systemctl --user disable --now super_v_clipboard
rm ~/.config/systemd/user/super_v_clipboard.service ~/.local/bin/super_v_clipboard
rm -r ~/.local/share/super_v_clipboard            # your saved history
sudo rm /etc/udev/rules.d/60-super-v-clipboard-uinput.rules
```
Then remove the keyboard shortcut in Settings.

## Contributing

Issues and pull requests are welcome. Please run `gofmt` and `go vet ./...` before sending changes.

## License

MIT. See [LICENSE](LICENSE).
