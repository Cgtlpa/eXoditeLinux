cat > /home/claude/vero/main.go << 'GOEOF'
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	cyan    = "\033[36m"
	blue    = "\033[34m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	red     = "\033[31m"
	magenta = "\033[35m"
	white   = "\033[97m"
)

func clear() { fmt.Print("\033[H\033[2J") }

func banner() {
	clear()
	fmt.Println()
	fmt.Println(bold + cyan + "  ██╗   ██╗███████╗██████╗  ██████╗ " + reset)
	fmt.Println(bold + cyan + "  ██║   ██║██╔════╝██╔══██╗██╔═══██╗" + reset)
	fmt.Println(bold + cyan + "  ██║   ██║█████╗  ██████╔╝██║   ██║" + reset)
	fmt.Println(bold + cyan + "  ╚██╗ ██╔╝██╔══╝  ██╔══██╗██║   ██║" + reset)
	fmt.Println(bold + cyan + "   ╚████╔╝ ███████╗██║  ██║╚██████╔╝" + reset)
	fmt.Println(bold + cyan + "    ╚═══╝  ╚══════╝╚═╝  ╚═╝ ╚═════╝ " + reset)
	fmt.Println()
	fmt.Println(dim + white + "  VeroLinux  ·  Simple. Fast. Yours." + reset)
	fmt.Println(dim + "  ─────────────────────────────────────────" + reset)
	fmt.Println()
}

func header(title string) {
	fmt.Printf("\n%s%s  %s%s\n", bold+cyan, "▶", title, reset)
	fmt.Println(dim + "  " + strings.Repeat("─", 40) + reset)
}

func info(msg string)    { fmt.Printf("  %s•%s %s\n", cyan, reset, msg) }
func success(msg string) { fmt.Printf("  %s✓%s %s\n", green, reset, msg) }
func warn(msg string)    { fmt.Printf("  %s!%s %s%s%s\n", yellow, reset, yellow, msg, reset) }
func fail(msg string)    { fmt.Printf("  %s✗%s %s%s%s\n", red, reset, red, msg, reset) }

func prompt(label string) string {
	fmt.Printf("  %s›%s %s: ", cyan, reset, label)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func promptDefault(label, def string) string {
	fmt.Printf("  %s›%s %s [%s%s%s]: ", cyan, reset, label, dim, def, reset)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return def
	}
	return val
}

func confirm(label string) bool {
	fmt.Printf("  %s?%s %s [%sy%s/%sN%s]: ", cyan, reset, label, green, reset, red, reset)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return ans == "y" || ans == "yes"
}

func spinRun(label string, cmd string, args ...string) error {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan error)
	go func() {
		c := exec.Command(cmd, args...)
		c.Stdout = nil
		c.Stderr = nil
		done <- c.Run()
	}()
	i := 0
	for {
		select {
		case err := <-done:
			fmt.Printf("\r  %s✓%s %s ... ", cyan, reset, label)
			if err != nil {
				fmt.Printf("%sfailed%s\n", red, reset)
			} else {
				fmt.Printf("%sdone%s\n", green, reset)
			}
			return err
		default:
			fmt.Printf("\r  %s%s%s %s ... ", cyan, frames[i%len(frames)], reset, label)
			time.Sleep(80 * time.Millisecond)
			i++
		}
	}
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func runSilent(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	return c.Run()
}

// ── network ───────────────────────────────────────────────────────────────────

func checkNetwork() {
	header("Network")
	// Start NetworkManager on the live ISO in case it isn't running yet
	runSilent("systemctl", "start", "NetworkManager")
	time.Sleep(2 * time.Second)

	if err := runSilent("ping", "-c", "1", "-W", "2", "archlinux.org"); err == nil {
		success("Network is up")
		return
	}
	warn("No network detected — launching nmtui to connect")
	fmt.Println()
	time.Sleep(1 * time.Second)
	run("nmtui")
	fmt.Println()
	if err := runSilent("ping", "-c", "1", "-W", "3", "archlinux.org"); err != nil {
		fail("Still no network. Check your connection and re-run Vero.")
		os.Exit(1)
	}
	success("Network connected")
}

// ── disk helpers ──────────────────────────────────────────────────────────────

func detectDisks() []string {
	out, err := exec.Command("lsblk", "-d", "-n", "-o", "NAME,TYPE").Output()
	if err != nil {
		return nil
	}
	var disks []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "disk" {
			disks = append(disks, "/dev/"+fields[0])
		}
	}
	return disks
}

func listDisks() {
	fmt.Println()
	run("lsblk", "-d", "-o", "NAME,SIZE,TYPE,MODEL")
	fmt.Println()
}

// ── packages ──────────────────────────────────────────────────────────────────

var basePkgs = []string{
	"base", "base-devel", "linux", "linux-firmware", "linux-headers",
	"networkmanager", "network-manager-applet", "grub", "efibootmgr", "os-prober",
	"sudo", "vim", "nano", "git", "curl", "wget",
	"htop", "fastfetch",
	"pipewire", "pipewire-pulse", "wireplumber",
	"zsh", "fish",
	"ttf-jetbrains-mono-nerd", "ttf-nerd-fonts-symbols",
	"grim", "slurp", "wl-clipboard",
	"firefox", "thunar", "alacritty",
	"ntfs-3g", "dosfstools", "exfatprogs",
	"sddm",
}

var dePkgs = map[string][]string{
	"hyprland": {
		"hyprland", "waybar", "wofi", "dunst",
		"hyprpaper", "xdg-desktop-portal-hyprland",
		"qt5-wayland", "qt6-wayland",
		"polkit-kde-agent",
	},
	"kde": {
		"plasma", "plasma-wayland-session",
		"kde-applications",
		"xdg-desktop-portal-kde",
	},
	"xfce4": {
		"xfce4", "xfce4-goodies",
		"xorg-server", "xorg-xinit",
	},
}

// ── fastfetch ─────────────────────────────────────────────────────────────────

var fastfetchLogo = `         {1}███             ███             ███             ██
{1}       ███░            ███░            ███░            ███░
{1}     ███░            ███░            ███░            ███░
{1}     ███░            ███░            ███░            ███░
{1}     ███░            ███░            ███░            ███░
{1}      ██░            ███░            ███░            ███░
{1}        ░            ███░            ███░            ███░
{1}                  ░░░             ░░░             ░░░
{1}             █$$\    $$\     ███             ███             ███
{1}      ███$$ |   $$ |  ███░            ███░            ███░
{1}     ███░ $$ |   $$ |█$$$$$$\   $$$$$$\█░ $$$$$$\    ███░
{1}      ██░   \$$\  $$  |$$  __$$\ $$  __$$\ $$  __$$\ ███░
{1}        ░      \$$\$$  /░$$$$$$$$ |$$ |█░\__|$$ /  $$ |█░
{1}                  \$$$  /  $$   ____|$$ |      $$ |  $$ |
{1}                   \$  /   \$$$$$$$\ $$ |      \$$$$$$  |
{1}          ██        ░░\_/     \_______|\__|       \______/
{1}          ░░░ ███             ███             ███             ███
{1}       ██░            ███░            ███░            ███░
{1}        ░            ███░            ███░            ███░
{1}                     ███░            ███░            ███░
{1}                     ███░            ███░            ███░            ██
{1}       ███░            ███░            ███░            ███░
{1}     ███░            ███░            ███░            ███░
{1}      ░░░             ░░░             ░░░             ░░░
{1}                 ███             ███             ███
{1}                  ███░            ███░            ███░
{1}                 ███░            ███░            ███░            ██
`

var fastfetchConfig = `{
  "$schema": "https://github.com/fastfetch-cli/fastfetch/raw/dev/doc/json_schema.json",
  "logo": {
    "source": "~/.config/fastfetch/logos/vero.txt",
    "color": { "1": "cyan", "2": "blue" }
  },
  "display": {
    "separator": "  ",
    "color": { "keys": "cyan", "title": "blue" }
  },
  "modules": [
    { "type": "title", "key": "", "format": "{user-name}@{host-name}" },
    "separator",
    { "type": "os",       "key": " OS",         "format": "VeroLinux" },
    { "type": "kernel",   "key": " Kernel" },
    { "type": "uptime",   "key": "󱐋 Uptime" },
    { "type": "packages", "key": " Packages" },
    { "type": "shell",    "key": " Shell" },
    { "type": "display",  "key": " Resolution" },
    { "type": "de",       "key": " DE/WM" },
    { "type": "terminal", "key": " Terminal" },
    { "type": "cpu",      "key": " CPU" },
    { "type": "gpu",      "key": "󰍛 GPU" },
    { "type": "memory",   "key": " Memory" },
    { "type": "disk",     "key": "󰋊 Disk" },
    "separator",
    { "type": "colors", "symbol": "block", "paddingLeft": 1 }
  ]
}
`

// ── os-release — makes EVERYTHING show VeroLinux ──────────────────────────────
var osRelease = `NAME="VeroLinux"
PRETTY_NAME="VeroLinux"
ID=verolinux
ID_LIKE=arch
BUILD_ID=rolling
ANSI_COLOR="36;1"
HOME_URL="https://github.com/Cgtlpa/VeroLinux"
BUG_REPORT_URL="https://github.com/Cgtlpa/VeroLinux/issues"
`

// ── config struct ─────────────────────────────────────────────────────────────

type Config struct {
	Disk     string
	EFIPart  string
	RootPart string
	SwapPart string
	Hostname string
	Username string
	Password string
	RootPass string
	Timezone string
	Locale   string
	DE       string
	Swap     bool
}

// ── gather config ─────────────────────────────────────────────────────────────

func gatherConfig() Config {
	var cfg Config

	banner()
	header("Disk Setup")
	listDisks()

	disks := detectDisks()
	suggested := ""
	for _, d := range disks {
		if strings.Contains(d, "nvme") {
			suggested = d
			break
		}
		if strings.Contains(d, "sda") && suggested == "" {
			suggested = d
		}
	}

	if len(disks) > 0 {
		info("Detected disks:")
		for _, d := range disks {
			fmt.Printf("    %s%s%s\n", cyan, d, reset)
		}
		fmt.Println()
	}

	if suggested != "" {
		cfg.Disk = promptDefault("Target disk", suggested)
	} else {
		cfg.Disk = prompt("Target disk (e.g. /dev/nvme0n1 or /dev/sda)")
	}

	if cfg.Disk == "" {
		fail("No disk selected.")
		os.Exit(1)
	}
	if _, err := os.Stat(cfg.Disk); err != nil {
		fail("Disk not found: " + cfg.Disk)
		os.Exit(1)
	}

	info("Vero will create: EFI (512M), optional SWAP (4G), ROOT (remaining)")
	cfg.Swap = confirm("Create a swap partition? (recommended)")

	header("System")
	cfg.Hostname = promptDefault("Hostname", "vero")
	cfg.Timezone = promptDefault("Timezone", "UTC")
	cfg.Locale = promptDefault("Locale", "en_US.UTF-8")

	header("User Account")
	cfg.Username = prompt("Username")
	fmt.Printf("  %s›%s Password (hidden): ", cyan, reset)
	run("stty", "-echo")
	cfg.Password = prompt("")
	run("stty", "echo")
	fmt.Println()
	fmt.Printf("  %s›%s Root password (hidden): ", cyan, reset)
	run("stty", "-echo")
	cfg.RootPass = prompt("")
	run("stty", "echo")
	fmt.Println()

	header("Desktop Environment")
	fmt.Println()
	fmt.Printf("  %s1%s  Hyprland   — wayland, tiling, minimal\n", cyan, reset)
	fmt.Printf("  %s2%s  KDE Plasma — full-featured, wayland\n", cyan, reset)
	fmt.Printf("  %s3%s  XFCE4      — lightweight, X11\n", cyan, reset)
	fmt.Printf("  %s4%s  None       — bare base install\n", cyan, reset)
	fmt.Println()
	deChoice := promptDefault("Choice", "1")

	switch deChoice {
	case "2":
		cfg.DE = "kde"
	case "3":
		cfg.DE = "xfce4"
	case "4":
		cfg.DE = "none"
	default:
		cfg.DE = "hyprland"
	}

	return cfg
}

// ── partition ─────────────────────────────────────────────────────────────────

func partition(cfg *Config) {
	header("Partitioning")

	disk := cfg.Disk
	isNVMe := strings.Contains(disk, "nvme")
	partSuffix := func(n int) string {
		if isNVMe {
			return fmt.Sprintf("%sp%d", disk, n)
		}
		return fmt.Sprintf("%s%d", disk, n)
	}

	if _, err := os.Stat(disk); err != nil {
		fail("Disk not found: " + disk)
		os.Exit(1)
	}

	runSilent("bash", "-c", "umount "+disk+"* 2>/dev/null || true")
	runSilent("bash", "-c", "swapoff "+disk+"* 2>/dev/null || true")

	info("Wiping disk: " + disk)

	if err := spinRun("Wiping partition table", "sgdisk", "--zap-all", disk); err != nil {
		fail("sgdisk failed — is the disk in use?")
		os.Exit(1)
	}
	if err := spinRun("Creating EFI partition (512M)", "sgdisk", "-n", "1:0:+512M", "-t", "1:ef00", disk); err != nil {
		fail("Failed to create EFI partition")
		os.Exit(1)
	}

	if cfg.Swap {
		if err := spinRun("Creating SWAP partition (4G)", "sgdisk", "-n", "2:0:+4G", "-t", "2:8200", disk); err != nil {
			fail("Failed to create SWAP partition")
			os.Exit(1)
		}
		if err := spinRun("Creating ROOT partition (remaining)", "sgdisk", "-n", "3:0:0", "-t", "3:8300", disk); err != nil {
			fail("Failed to create ROOT partition")
			os.Exit(1)
		}
		cfg.EFIPart = partSuffix(1)
		cfg.SwapPart = partSuffix(2)
		cfg.RootPart = partSuffix(3)
	} else {
		if err := spinRun("Creating ROOT partition (remaining)", "sgdisk", "-n", "2:0:0", "-t", "2:8300", disk); err != nil {
			fail("Failed to create ROOT partition")
			os.Exit(1)
		}
		cfg.EFIPart = partSuffix(1)
		cfg.SwapPart = ""
		cfg.RootPart = partSuffix(2)
	}

	time.Sleep(1 * time.Second)
	runSilent("partprobe", disk)
	time.Sleep(1 * time.Second)

	if err := spinRun("Formatting EFI (FAT32)", "mkfs.fat", "-F32", cfg.EFIPart); err != nil {
		fail("mkfs.fat failed on " + cfg.EFIPart)
		os.Exit(1)
	}
	if cfg.Swap {
		if err := spinRun("Formatting SWAP", "mkswap", cfg.SwapPart); err != nil {
			fail("mkswap failed on " + cfg.SwapPart)
			os.Exit(1)
		}
	}
	if err := spinRun("Formatting ROOT (ext4)", "mkfs.ext4", "-F", cfg.RootPart); err != nil {
		fail("mkfs.ext4 failed on " + cfg.RootPart)
		os.Exit(1)
	}

	if err := spinRun("Mounting ROOT", "mount", cfg.RootPart, "/mnt"); err != nil {
		fail("Could not mount " + cfg.RootPart)
		os.Exit(1)
	}
	runSilent("mkdir", "-p", "/mnt/boot/efi")
	if err := spinRun("Mounting EFI", "mount", cfg.EFIPart, "/mnt/boot/efi"); err != nil {
		fail("Could not mount EFI partition")
		os.Exit(1)
	}
	if cfg.Swap {
		spinRun("Enabling SWAP", "swapon", cfg.SwapPart)
	}

	success("Partitioning complete")
}

// ── install base ──────────────────────────────────────────────────────────────

func installBase(cfg Config) {
	header("Installing Base System")

	allPkgs := append([]string{}, basePkgs...)
	if cfg.DE != "none" {
		allPkgs = append(allPkgs, dePkgs[cfg.DE]...)
	}

	info(fmt.Sprintf("Installing %d packages via pacstrap ...", len(allPkgs)))
	fmt.Println()

	args := append([]string{"/mnt"}, allPkgs...)
	if err := run("pacstrap", args...); err != nil {
		fail("pacstrap failed. Check your network connection.")
		os.Exit(1)
	}

	success("Base system installed")
}

// ── fstab ─────────────────────────────────────────────────────────────────────

func generateFstab() {
	header("Generating fstab")
	out, _ := exec.Command("genfstab", "-U", "/mnt").Output()
	f, _ := os.OpenFile("/mnt/etc/fstab", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	f.Write(out)
	f.Close()
	success("fstab written")
}

// ── chroot script ─────────────────────────────────────────────────────────────

func chrootScript(cfg Config) string {
	// SDDM is used for ALL desktop environments
	deEnable := "# No desktop environment selected"
	sddmSession := ""
	if cfg.DE != "none" {
		deEnable = "systemctl enable sddm"
	}
	// For Hyprland: create a wayland session file so SDDM can launch it
	if cfg.DE == "hyprland" {
		sddmSession = `
mkdir -p /usr/share/wayland-sessions
cat > /usr/share/wayland-sessions/hyprland.desktop << 'SEOF'
[Desktop Entry]
Name=Hyprland
Comment=An intelligent dynamic tiling Wayland compositor
Exec=Hyprland
Type=Application
SEOF
`
	}

	fastfetchSetup := fmt.Sprintf(`mkdir -p /home/%s/.config/fastfetch/logos
cat > /home/%s/.config/fastfetch/config.jsonc << 'FFEOF'
%s
FFEOF
cat > /home/%s/.config/fastfetch/logos/vero.txt << 'LOGOEOF'
%s
LOGOEOF`, cfg.Username, cfg.Username, fastfetchConfig, cfg.Username, fastfetchLogo)

	return fmt.Sprintf(`#!/bin/bash
set -e

# Timezone
ln -sf /usr/share/zoneinfo/%s /etc/localtime
hwclock --systohc

# Locale
echo "%s UTF-8" >> /etc/locale.gen
locale-gen
echo "LANG=%s" > /etc/locale.conf

# Hostname
echo "%s" > /etc/hostname
cat >> /etc/hosts << 'HEOF'
127.0.0.1   localhost
::1         localhost
127.0.1.1   %s.localdomain %s
HEOF

# VeroLinux identity — overrides Arch everywhere (fastfetch, apps, GRUB)
cat > /etc/os-release << 'OEOF'
%s
OEOF

# Users
echo "root:%s" | chpasswd
useradd -m -G wheel,audio,video,storage,optical -s /bin/bash %s
echo "%s:%s" | chpasswd
echo "%%wheel ALL=(ALL:ALL) ALL" >> /etc/sudoers

# Initramfs
mkinitcpio -P

# GRUB — branded as VeroLinux
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=VeroLinux
sed -i 's/GRUB_DISTRIBUTOR=.*/GRUB_DISTRIBUTOR="VeroLinux"/' /etc/default/grub
sed -i 's/GRUB_TIMEOUT=5/GRUB_TIMEOUT=3/' /etc/default/grub
sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT=.*/GRUB_CMDLINE_LINUX_DEFAULT="loglevel=3 quiet splash"/' /etc/default/grub
grub-mkconfig -o /boot/grub/grub.cfg

# Services
systemctl enable NetworkManager
systemctl enable fstrim.timer
systemctl enable bluetooth 2>/dev/null || true

# Desktop + SDDM
%s
%s

# Fastfetch config + custom logo
%s

# Auto-launch fastfetch on terminal open
echo -e "\n# VeroLinux\nfastfetch" >> /home/%s/.bashrc
echo -e "\n# VeroLinux\nfastfetch" >> /home/%s/.zshrc 2>/dev/null || true
chown -R %s:%s /home/%s

echo "CHROOT_DONE"
`,
		cfg.Timezone,
		cfg.Locale, cfg.Locale,
		cfg.Hostname,
		cfg.Hostname, cfg.Hostname,
		osRelease,
		cfg.RootPass,
		cfg.Username,
		cfg.Username, cfg.Password,
		deEnable,
		sddmSession,
		fastfetchSetup,
		cfg.Username, cfg.Username,
		cfg.Username, cfg.Username, cfg.Username,
	)
}

func configureSystem(cfg Config) {
	header("Configuring System")

	script := chrootScript(cfg)
	if err := os.WriteFile("/mnt/vero-chroot.sh", []byte(script), 0755); err != nil {
		fail("Could not write chroot script: " + err.Error())
		os.Exit(1)
	}

	info("Entering chroot ...")
	fmt.Println()
	if err := run("arch-chroot", "/mnt", "/bin/bash", "/vero-chroot.sh"); err != nil {
		fail("Chroot configuration failed: " + err.Error())
		os.Exit(1)
	}
	os.Remove("/mnt/vero-chroot.sh")
	success("System configured")
}

// ── finish ────────────────────────────────────────────────────────────────────

func finish(cfg Config) {
	header("Finishing Up")
	spinRun("Unmounting filesystems", "umount", "-R", "/mnt")
	if cfg.Swap {
		runSilent("swapoff", cfg.SwapPart)
	}

	fmt.Println()
	fmt.Println(bold + cyan + "  ┌─────────────────────────────────────────┐" + reset)
	fmt.Println(bold + cyan + "  │                                         │" + reset)
	fmt.Println(bold + cyan + "  │   " + green + "VeroLinux installed successfully! 🎉  " + cyan + "│" + reset)
	fmt.Println(bold + cyan + "  │                                         │" + reset)
	fmt.Println(bold + cyan + "  │   " + white + "Remove install media and reboot       " + cyan + "│" + reset)
	fmt.Println(bold + cyan + "  │                                         │" + reset)
	fmt.Println(bold + cyan + "  └─────────────────────────────────────────┘" + reset)
	fmt.Println()
	fmt.Printf("  %sHostname :%s  %s\n", dim, reset, cfg.Hostname)
	fmt.Printf("  %sUsername :%s  %s\n", dim, reset, cfg.Username)
	fmt.Printf("  %sDesktop  :%s  %s\n", dim, reset, cfg.DE)
	fmt.Println()

	if confirm("Reboot now?") {
		run("reboot")
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	if os.Getuid() != 0 {
		fmt.Println(red + "  Vero must be run as root." + reset)
		os.Exit(1)
	}

	banner()
	fmt.Println(bold + "  Welcome to VeroLinux — the Arch-based installer" + reset)
	fmt.Println(dim + "  This will guide you through a complete installation." + reset)
	fmt.Println()

	if !confirm("Continue with installation?") {
		fmt.Println("  Aborted.")
		os.Exit(0)
	}

	checkNetwork()
	cfg := gatherConfig()

	banner()
	header("Installation Summary")
	fmt.Printf("  Disk      : %s%s%s\n", cyan, cfg.Disk, reset)
	fmt.Printf("  Hostname  : %s%s%s\n", cyan, cfg.Hostname, reset)
	fmt.Printf("  Username  : %s%s%s\n", cyan, cfg.Username, reset)
	fmt.Printf("  Timezone  : %s%s%s\n", cyan, cfg.Timezone, reset)
	fmt.Printf("  Locale    : %s%s%s\n", cyan, cfg.Locale, reset)
	fmt.Printf("  Desktop   : %s%s%s\n", cyan, cfg.DE, reset)
	fmt.Printf("  Swap      : %s%v%s\n", cyan, cfg.Swap, reset)
	fmt.Println()

	if !confirm("Proceed? (THIS WILL WIPE " + cfg.Disk + ")") {
		fmt.Println("  Aborted.")
		os.Exit(0)
	}

	partition(&cfg)
	installBase(cfg)
	generateFstab()
	configureSystem(cfg)
	finish(cfg)
}
GOEOF
echo "WRITTEN"
