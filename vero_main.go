package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ── ANSI colors ──────────────────────────────────────────────────────────────
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

// ── helpers ───────────────────────────────────────────────────────────────────

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
	fmt.Println(dim + white + "  Arch-based Linux  ·  Simple. Fast. Yours." + reset)
	fmt.Println(dim + "  ─────────────────────────────────────────" + reset)
	fmt.Println()
}

func header(title string) {
	fmt.Printf("\n%s%s  %s%s\n", bold+cyan, "▶", title, reset)
	fmt.Println(dim + "  " + strings.Repeat("─", 40) + reset)
}

func info(msg string) {
	fmt.Printf("  %s•%s %s\n", cyan, reset, msg)
}

func success(msg string) {
	fmt.Printf("  %s✓%s %s\n", green, reset, msg)
}

func warn(msg string) {
	fmt.Printf("  %s!%s %s%s%s\n", yellow, reset, yellow, msg, reset)
}

func fail(msg string) {
	fmt.Printf("  %s✗%s %s%s%s\n", red, reset, red, msg, reset)
}

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
			fmt.Printf("\r  %s%s%s %s ... ", cyan, "✓", reset, label)
			if err != nil {
				fmt.Printf("%s%s%s\n", red, "failed", reset)
			} else {
				fmt.Printf("%s%s%s\n", green, "done", reset)
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

// ── wifi check ───────────────────────────────────────────────────────────────

func checkNetwork() {
	header("Network")
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

// ── disk helpers ─────────────────────────────────────────────────────────────

func listDisks() {
	fmt.Println()
	run("lsblk", "-d", "-o", "NAME,SIZE,TYPE,MODEL")
	fmt.Println()
}

// ── locale / timezone helpers ────────────────────────────────────────────────

func listTimezones() {
	run("timedatectl", "list-timezones")
}

// ── pacstrap packages ─────────────────────────────────────────────────────────

var basePkgs = []string{
	"base", "base-devel", "linux", "linux-firmware", "linux-headers",
	"networkmanager", "grub", "efibootmgr", "os-prober",
	"sudo", "vim", "nano", "git", "curl", "wget",
	"htop", "neofetch", "fastfetch",
	"pipewire", "pipewire-pulse", "wireplumber",
	"zsh", "fish",
	"ttf-jetbrains-mono-nerd", "ttf-nerd-fonts-symbols",
	"grim", "slurp", "wl-clipboard",
	"firefox", "thunar", "alacritty",
	"ntfs-3g", "dosfstools", "exfatprogs",
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
		"kde-applications", "sddm",
		"xdg-desktop-portal-kde",
	},
	"xfce4": {
		"xfce4", "xfce4-goodies",
		"lightdm", "lightdm-gtk-greeter",
		"xorg-server", "xorg-xinit",
	},
}

// ── fastfetch config ──────────────────────────────────────────────────────────

var fastfetchConfig = `{
  "$schema": "https://github.com/fastfetch-cli/fastfetch/raw/dev/doc/json_schema.json",
  "logo": {
    "source": "vero",
    "color": {
      "1": "cyan",
      "2": "blue"
    }
  },
  "display": {
    "separator": "  ",
    "color": {
      "keys": "cyan",
      "title": "blue"
    }
  },
  "modules": [
    {
      "type": "title",
      "key": "",
      "format": "{user-name}@{host-name}"
    },
    "separator",
    { "type": "os",      "key": " OS" },
    { "type": "kernel",  "key": " Kernel" },
    { "type": "uptime",  "key": "󱐋 Uptime" },
    { "type": "packages","key": " Packages" },
    { "type": "shell",   "key": " Shell" },
    { "type": "display", "key": " Resolution" },
    { "type": "de",      "key": " DE/WM" },
    { "type": "terminal","key": " Terminal" },
    { "type": "cpu",     "key": " CPU" },
    { "type": "gpu",     "key": "󰍛 GPU" },
    { "type": "memory",  "key": " Memory" },
    { "type": "disk",    "key": "󰋊 Disk" },
    "separator",
    { "type": "colors", "symbol": "block", "paddingLeft": 1 }
  ]
}
`

var fastfetchLogo = `// Vero logo for fastfetch
// Place in ~/.config/fastfetch/logos/vero.txt
{1}██╗   ██╗███████╗██████╗  ██████╗ 
{1}██║   ██║██╔════╝██╔══██╗██╔═══██╗
{1}██║   ██║█████╗  ██████╔╝██║   ██║
{1}╚██╗ ██╔╝██╔══╝  ██╔══██╗██║   ██║
{1} ╚████╔╝ ███████╗██║  ██║╚██████╔╝
{1}  ╚═══╝  ╚══════╝╚═╝  ╚═╝ ╚═════╝ 
`

// ── install steps ─────────────────────────────────────────────────────────────

type Config struct {
	Disk       string
	EFIPart    string
	RootPart   string
	SwapPart   string
	Hostname   string
	Username   string
	Password   string
	RootPass   string
	Timezone   string
	Locale     string
	DE         string
	Bootloader string
	Swap       bool
}

func gatherConfig() Config {
	var cfg Config

	banner()
	header("Disk Setup")
	listDisks()

	cfg.Disk = prompt("Target disk (e.g. /dev/sda or /dev/nvme0n1)")
	if cfg.Disk == "" {
		fail("No disk selected. Exiting.")
		os.Exit(1)
	}

	info("Vero will create 3 partitions: EFI (512M), SWAP (optional), ROOT (remaining)")
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
	fmt.Printf("  %s1%s  Hyprland  — wayland, tiling, minimal\n", cyan, reset)
	fmt.Printf("  %s2%s  KDE Plasma — full-featured, wayland\n", cyan, reset)
	fmt.Printf("  %s3%s  XFCE4     — lightweight, X11\n", cyan, reset)
	fmt.Printf("  %s4%s  None      — bare base install\n", cyan, reset)
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

	info("Wiping disk: " + disk)

	// Build sgdisk commands
	spinRun("Wiping partition table", "sgdisk", "--zap-all", disk)
	spinRun("Creating EFI partition (512M)", "sgdisk", "-n", "1:0:+512M", "-t", "1:ef00", disk)

	if cfg.Swap {
		spinRun("Creating SWAP partition (4G)", "sgdisk", "-n", "2:0:+4G", "-t", "2:8200", disk)
		spinRun("Creating ROOT partition (remaining)", "sgdisk", "-n", "3:0:0", "-t", "3:8300", disk)
		cfg.EFIPart = partSuffix(1)
		cfg.SwapPart = partSuffix(2)
		cfg.RootPart = partSuffix(3)
	} else {
		spinRun("Creating ROOT partition (remaining)", "sgdisk", "-n", "2:0:0", "-t", "2:8300", disk)
		cfg.EFIPart = partSuffix(1)
		cfg.SwapPart = ""
		cfg.RootPart = partSuffix(2)
	}

	// Format
	spinRun("Formatting EFI (FAT32)", "mkfs.fat", "-F32", cfg.EFIPart)
	if cfg.Swap {
		spinRun("Formatting SWAP", "mkswap", cfg.SwapPart)
	}
	spinRun("Formatting ROOT (ext4)", "mkfs.ext4", "-F", cfg.RootPart)

	// Mount
	spinRun("Mounting ROOT", "mount", cfg.RootPart, "/mnt")
	runSilent("mkdir", "-p", "/mnt/boot/efi")
	spinRun("Mounting EFI", "mount", cfg.EFIPart, "/mnt/boot/efi")
	if cfg.Swap {
		spinRun("Enabling SWAP", "swapon", cfg.SwapPart)
	}

	success("Partitioning complete")
}

// ── install base ─────────────────────────────────────────────────────────────

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

// ── chroot config ─────────────────────────────────────────────────────────────

func chrootScript(cfg Config) string {
	deEnable := ""
	if cfg.DE == "kde" {
		deEnable = "systemctl enable sddm"
	} else if cfg.DE == "xfce4" {
		deEnable = "systemctl enable lightdm"
	} else if cfg.DE == "hyprland" {
		deEnable = "# Hyprland: start via ~/.config/hypr/autostart or display-manager of choice"
	}

	fastfetchSetup := fmt.Sprintf(`
mkdir -p /home/%s/.config/fastfetch/logos
cat > /home/%s/.config/fastfetch/config.jsonc << 'FFEOF'
%s
FFEOF
cat > /home/%s/.config/fastfetch/logos/vero.txt << 'LOGOEOF'
%s
LOGOEOF
`, cfg.Username, cfg.Username, fastfetchConfig, cfg.Username, fastfetchLogo)

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
cat >> /etc/hosts << EOF
127.0.0.1   localhost
::1         localhost
127.0.1.1   %s.localdomain %s
EOF

# Users
echo "root:%s" | chpasswd
useradd -m -G wheel,audio,video,storage,optical -s /bin/bash %s
echo "%s:%s" | chpasswd
echo "%%wheel ALL=(ALL:ALL) ALL" >> /etc/sudoers

# Initramfs
mkinitcpio -P

# GRUB bootloader
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=VERO
sed -i 's/GRUB_TIMEOUT=5/GRUB_TIMEOUT=3/' /etc/default/grub
sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT="loglevel=3 quiet"/GRUB_CMDLINE_LINUX_DEFAULT="loglevel=3 quiet splash"/' /etc/default/grub
grub-mkconfig -o /boot/grub/grub.cfg

# Services
systemctl enable NetworkManager
systemctl enable fstrim.timer
systemctl enable bluetooth 2>/dev/null || true

# Desktop environment
%s

# Fastfetch config
%s

# Shell tweaks — add fastfetch to .bashrc and .zshrc
echo -e "\n# Vero greeting\nfastfetch" >> /home/%s/.bashrc
echo -e "\n# Vero greeting\nfastfetch" >> /home/%s/.zshrc 2>/dev/null || true
chown -R %s:%s /home/%s

echo "CHROOT_DONE"
`,
		cfg.Timezone,
		cfg.Locale, cfg.Locale,
		cfg.Hostname,
		cfg.Hostname, cfg.Hostname,
		cfg.RootPass,
		cfg.Username,
		cfg.Username, cfg.Password,
		deEnable,
		fastfetchSetup,
		cfg.Username, cfg.Username,
		cfg.Username, cfg.Username, cfg.Username,
	)
}

func configureSystem(cfg Config) {
	header("Configuring System")

	script := chrootScript(cfg)
	err := os.WriteFile("/mnt/vero-chroot.sh", []byte(script), 0755)
	if err != nil {
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

// ── unmount & finish ──────────────────────────────────────────────────────────

func finish(cfg Config) {
	header("Finishing Up")
	spinRun("Unmounting filesystems", "umount", "-R", "/mnt")
	if cfg.Swap {
		runSilent("swapoff", cfg.SwapPart)
	}

	fmt.Println()
	fmt.Println(bold + cyan + "  ┌─────────────────────────────────────────┐" + reset)
	fmt.Println(bold + cyan + "  │                                         │" + reset)
	fmt.Println(bold + cyan + "  │   " + green + "Vero installed successfully! 🎉       " + cyan + "│" + reset)
	fmt.Println(bold + cyan + "  │                                         │" + reset)
	fmt.Println(bold + cyan + "  │  " + white + " Remove your install media and reboot  " + cyan + " │" + reset)
	fmt.Println(bold + cyan + "  │                                         │" + reset)
	fmt.Println(bold + cyan + "  └─────────────────────────────────────────┘" + reset)
	fmt.Println()
	fmt.Printf("  %s Hostname:%s  %s\n", dim, reset, cfg.Hostname)
	fmt.Printf("  %s Username:%s  %s\n", dim, reset, cfg.Username)
	fmt.Printf("  %s      DE :%s  %s\n", dim, reset, cfg.DE)
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

	fmt.Println(bold + "  Welcome to Vero — the Arch-based installer" + reset)
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
