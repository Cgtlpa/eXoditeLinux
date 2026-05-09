package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/term"
)


const (
	distroName      = "eXodite"
	distroNameLower = "exodite"
	distroID        = "exodite"
)


const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[1;32m"
	ansiYellow = "\033[1;33m"
	ansiCyan   = "\033[1;36m"
	ansiPurple = "\033[1;35m"
)


type DiskLayout struct {
	EFISize  string
	SwapSize string
	RootSize string 
	HomePart bool  
}

type Config struct {
	Disk          string
	DiskSizeBytes uint64
	Layout        DiskLayout
	Kernel        string
	GPU           string
	Desktop       string
	Hostname      string
	Username      string
	Password      string
	RootPass      string
	Timezone      string
	Keymap        string
	Locale        string
	InstallYay    string
}



func main() {
	if os.Getuid() != 0 {
		die("Root privileges required.")
	}

	setupNetwork()
	header(distroName + " Linux Installer")

	cfg := gatherConfig()

	if err := runInstaller(cfg); err != nil {
		fmt.Printf("\n"+ansiRed+"[!] Installation failed: %v"+ansiReset+"\n", err)
		cleanup()
		os.Exit(1)
	}

	header("Installation complete! Remove installation media and reboot.")
}


func runInstaller(cfg Config) error {
	
	if cfg.Kernel == "linux-cachyos" {
		if err := setupCachyLive(); err != nil {
			return fmt.Errorf("CachyOS live setup: %w", err)
		}
	}

	steps := []struct {
		name string
		fn   func(Config) error
	}{
		{"Partitioning disk", partition},
		{"Installing base system", installBase},
		{"Configuring system", configure},
	}

	for _, s := range steps {
		header(s.name)
		if err := s.fn(cfg); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

func cleanup() {
	fmt.Println("[*] Unmounting filesystems...")
	exec.Command("umount", "-R", "/mnt").Run()
}


func header(msg string) {
	fmt.Printf("\n"+ansiPurple+"=== %s ==="+ansiReset+"\n\n", msg)
}

func die(msg string) {
	fmt.Printf(ansiRed+"[!] %s"+ansiReset+"\n", msg)
	os.Exit(1)
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func runSilent(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func spinRun(msg, cmd string, args ...string) error {
	fmt.Printf(ansiCyan+"[*]"+ansiReset+" %s...", msg)
	if err := runSilent(cmd, args...); err != nil {
		fmt.Printf("\r"+ansiRed+"[✗]"+ansiReset+" %s... Failed!\n", msg)
		return err
	}
	fmt.Printf("\r"+ansiGreen+"[✓]"+ansiReset+" %s\n", msg)
	return nil
}



func menuSelect(title string, options []string) string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		die("Cannot open /dev/tty: " + err.Error())
	}
	defer tty.Close()

	fd := int(tty.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		die("Cannot set raw terminal: " + err.Error())
	}
	defer term.Restore(fd, oldState)

	const visibleRows = 10
	selected := 0
	scrollOffset := 0
	buf := make([]byte, 3)

	for {
		if selected < scrollOffset {
			scrollOffset = selected
		}
		if selected >= scrollOffset+visibleRows {
			scrollOffset = selected - visibleRows + 1
		}

		fmt.Fprint(tty, "\033[2J\033[H")
		fmt.Fprintf(tty, "\n"+ansiPurple+"=== %s ==="+ansiReset+"\n\n", title)

		if scrollOffset > 0 {
			fmt.Fprintf(tty, "     \033[90m↑ %d more\033[0m\n", scrollOffset)
		} else {
			fmt.Fprint(tty, "\n")
		}

		end := scrollOffset + visibleRows
		if end > len(options) {
			end = len(options)
		}
		for i := scrollOffset; i < end; i++ {
			if i == selected {
				fmt.Fprintf(tty, ansiGreen+"   → %s"+ansiReset+"\n", options[i])
			} else {
				fmt.Fprintf(tty, "     %s\n", options[i])
			}
		}

		remaining := len(options) - end
		if remaining > 0 {
			fmt.Fprintf(tty, "     \033[90m↓ %d more\033[0m\n", remaining)
		} else {
			fmt.Fprint(tty, "\n")
		}

		n, _ := tty.Read(buf)
		if n == 0 {
			continue
		}

		switch {
		case n == 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 65: 
			if selected > 0 {
				selected--
			}
		case n == 3 && buf[0] == 27 && buf[1] == 91 && buf[2] == 66: 
			if selected < len(options)-1 {
				selected++
			}
		case buf[0] == 10 || buf[0] == 13: 
			return options[selected]
		}
	}
}

func prompt(msg, def string, mask bool) string {
	if def != "" {
		fmt.Printf(ansiYellow+"?"+ansiReset+" %s [%s]: ", msg, def)
	} else {
		fmt.Printf(ansiYellow+"?"+ansiReset+" %s: ", msg)
	}

	if mask {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil || len(b) == 0 {
			return def
		}
		return string(b)
	}

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			return def
		}
		return line
	}
	return def
}


func setupNetwork() {
	runSilent("systemctl", "start", "NetworkManager")
	if exec.Command("ping", "-c", "1", "-W", "3", "8.8.8.8").Run() != nil {
		if strings.ToLower(prompt("Network offline. Open nmtui?", "y", false)) == "y" {
			run("nmtui")
		}
	}
}



func setupCachyLive() error {
	fmt.Println("[*] Configuring CachyOS repository on live system...")

	const cachyKey = "F1656F40D7482129"
	if err := spinRun("Receiving CachyOS signing key", "pacman-key", "--recv-keys", cachyKey); err != nil {
		return err
	}
	if err := spinRun("Locally signing CachyOS key", "pacman-key", "--lsign-key", cachyKey); err != nil {
		return err
	}

	conf, err := os.ReadFile("/etc/pacman.conf")
	if err != nil {
		return fmt.Errorf("reading pacman.conf: %w", err)
	}
	if !strings.Contains(string(conf), "[cachyos]") {
		f, err := os.OpenFile("/etc/pacman.conf", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening pacman.conf: %w", err)
		}
		_, werr := f.WriteString(cachyRepoBlock)
		f.Close()
		if werr != nil {
			return fmt.Errorf("writing pacman.conf: %w", werr)
		}
	}

	if err := spinRun("Syncing package databases", "pacman", "-Sy", "--noconfirm"); err != nil {
		return err
	}
	return spinRun("Installing CachyOS keyring", "pacman", "-S", "--noconfirm", "cachyos-keyring")
}


const cachyRepoBlock = `
[cachyos]
Server = https://mirror.cachyos.org/$repo/$arch
SigLevel = Required DatabaseOptional
`



var timezones = []string{
	"Europe/Berlin", "Europe/London", "Europe/Paris", "Europe/Rome",
	"America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles",
	"Asia/Tokyo", "Asia/Shanghai", "Asia/Kolkata", "Australia/Sydney",
	"UTC",
}

var keymaps = []string{
	"us", "de", "uk", "fr", "es", "it", "pt", "ru", "pl", "nl", "colemak",
}

var locales = []string{
	"en_US.UTF-8", "en_GB.UTF-8", "de_DE.UTF-8", "fr_FR.UTF-8",
	"es_ES.UTF-8", "it_IT.UTF-8", "pt_PT.UTF-8", "ru_RU.UTF-8",
}


type diskInfo struct {
	path      string
	sizeLabel string
	model     string
	sizeBytes uint64
}


func detectDisks() []diskInfo {
	out, err := exec.Command("lsblk", "-d", "-n", "-o", "NAME,SIZE,MODEL").Output()
	if err != nil {
		die("Could not detect disks.")
	}

	var disks []diskInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		name := fields[0]
		if strings.HasPrefix(name, "loop") {
			continue
		}
		d := diskInfo{
			path:      "/dev/" + name,
			sizeLabel: fields[1],
			model:     strings.Join(fields[2:], " "),
		}
		if raw, err := exec.Command("lsblk", "-b", "-d", "-n", "-o", "SIZE", d.path).Output(); err == nil {
			if b, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64); err == nil {
				d.sizeBytes = b
			}
		}
		disks = append(disks, d)
	}
	if len(disks) == 0 {
		die("No target disks found.")
	}
	return disks
}

func gatherConfig() Config {
	var cfg Config

	disks := detectDisks()
	options := make([]string, len(disks))
	for i, d := range disks {
		options[i] = fmt.Sprintf("%s  (%s %s)", d.path, d.sizeLabel, d.model)
	}

	cfg.Keymap = strings.Fields(menuSelect("Keyboard Layout", keymaps))[0]
	runSilent("loadkeys", cfg.Keymap)

	cfg.Timezone = strings.Fields(menuSelect("Timezone", timezones))[0]
	cfg.Locale = strings.Fields(menuSelect("Locale", locales))[0]

	diskChoice := menuSelect("Target Disk  (↑↓ to move, Enter to select)", options)
	chosenPath := strings.Fields(diskChoice)[0]
	for _, d := range disks {
		if d.path == chosenPath {
			cfg.Disk = d.path
			cfg.DiskSizeBytes = d.sizeBytes
			break
		}
	}

	confirm := menuSelect(
		fmt.Sprintf("⚠  ALL DATA ON %s WILL BE DESTROYED — continue?", cfg.Disk),
		[]string{"No — go back", "Yes — wipe and install"},
	)
	if confirm != "Yes — wipe and install" {
		fmt.Println("Aborted.")
		os.Exit(0)
	}

	cfg.Layout = gatherDiskLayout(cfg.DiskSizeBytes)

	cfg.Kernel = menuSelect("Kernel", []string{"linux", "linux-zen", "linux-cachyos"})
	cfg.GPU = menuSelect("Graphics Driver", []string{
		"NVIDIA (proprietary)",
		"NVIDIA 580xx (AUR, DKMS)",
		"Open Source (Intel / AMD / Nouveau)",
		"None (No extra drivers)",
	})
	cfg.Desktop = menuSelect("Desktop Environment", []string{
		"KDE Plasma",
		"XFCE4",
		"Hyprland",
		"None (TTY only)",
	})
	cfg.InstallYay = menuSelect("Install Yay (AUR helper)?", []string{"Yes", "No"})

	fmt.Print("\033[2J\033[H")
	header("User Accounts")

	cfg.Hostname = prompt("Hostname", distroNameLower, false)
	cfg.Username = prompt("Username", "user", false)

	for {
		cfg.Password = prompt("User password", "", true)
		if c := prompt("Confirm user password", "", true); cfg.Password == c {
			break
		}
		fmt.Println(ansiRed + "[!] Passwords do not match, try again." + ansiReset)
	}

	for {
		cfg.RootPass = prompt("Root password", "", true)
		if c := prompt("Confirm root password", "", true); cfg.RootPass == c {
			break
		}
		fmt.Println(ansiRed + "[!] Passwords do not match, try again." + ansiReset)
	}

	return cfg
}


func gatherDiskLayout(diskBytes uint64) DiskLayout {
	choice := menuSelect("Disk Partition Layout", []string{
		"Auto  (512 MiB EFI, no swap, rest → root)",
		"Custom EFI size, no swap",
		"Custom EFI size + swap partition",
		"Split disk  (separate /home partition)",
	})

	switch choice {
	case "Auto  (512 MiB EFI, no swap, rest → root)":
		return DiskLayout{EFISize: "512M"}

	case "Custom EFI size, no swap":
		efi := prompt("EFI partition size (e.g. 512M, 1G)", "512M", false)
		return DiskLayout{EFISize: efi}

	case "Custom EFI size + swap partition":
		efi := prompt("EFI partition size (e.g. 512M, 1G)", "512M", false)
		swap := prompt("Swap partition size (e.g. 4G, 8G)", "4G", false)
		root := prompt("Root partition size — leave blank to use remaining space", "", false)
		return DiskLayout{EFISize: efi, SwapSize: swap, RootSize: root}

	default:
		efi := prompt("EFI partition size (e.g. 512M, 1G)", "512M", false)

	
		defRoot := "50G"
		if diskBytes > 0 {
			diskGB := diskBytes / 1024 / 1024 / 1024
			if diskGB <= 40 {
				defRoot = fmt.Sprintf("%dG", diskGB/2)
			}
		}
		root := prompt("Root partition size (rest goes to /home)", defRoot, false)
		return DiskLayout{EFISize: efi, RootSize: root, HomePart: true}
	}
}


func partPrefix(disk string) string {
	if strings.Contains(disk, "nvme") || strings.Contains(disk, "mmcblk") {
		return disk + "p"
	}
	return disk
}

func partition(cfg Config) error {
	disk := cfg.Disk
	l := cfg.Layout
	p := partPrefix(disk)

	if err := spinRun("Wiping partition table on "+disk, "sgdisk", "-Z", disk); err != nil {
		return err
	}

	partNum := 1

	
	if err := spinRun(
		fmt.Sprintf("Creating EFI partition (%s)", l.EFISize),
		"sgdisk", "-n", fmt.Sprintf("%d:0:+%s", partNum, l.EFISize),
		"-t", fmt.Sprintf("%d:ef00", partNum), disk,
	); err != nil {
		return err
	}
	efi := fmt.Sprintf("%s%d", p, partNum)
	partNum++

	
	var swapPart string
	if l.SwapSize != "" {
		if err := spinRun(
			fmt.Sprintf("Creating swap partition (%s)", l.SwapSize),
			"sgdisk", "-n", fmt.Sprintf("%d:0:+%s", partNum, l.SwapSize),
			"-t", fmt.Sprintf("%d:8200", partNum), disk,
		); err != nil {
			return err
		}
		swapPart = fmt.Sprintf("%s%d", p, partNum)
		partNum++
	}


	rootEnd, rootLabel := "0", "remainder"
	if l.RootSize != "" {
		rootEnd = "+" + l.RootSize
		rootLabel = l.RootSize
	}
	if err := spinRun(
		fmt.Sprintf("Creating root partition (%s)", rootLabel),
		"sgdisk", "-n", fmt.Sprintf("%d:0:%s", partNum, rootEnd),
		"-t", fmt.Sprintf("%d:8300", partNum), disk,
	); err != nil {
		return err
	}
	root := fmt.Sprintf("%s%d", p, partNum)
	partNum++


	var homePart string
	if l.HomePart {
		if err := spinRun("Creating home partition (remainder)",
			"sgdisk", "-n", fmt.Sprintf("%d:0:0", partNum),
			"-t", fmt.Sprintf("%d:8300", partNum), disk,
		); err != nil {
			return err
		}
		homePart = fmt.Sprintf("%s%d", p, partNum)
	}


	if err := spinRun("Formatting EFI (FAT32)", "mkfs.fat", "-F32", efi); err != nil {
		return err
	}
	if swapPart != "" {
		if err := spinRun("Formatting swap", "mkswap", swapPart); err != nil {
			return err
		}
	}
	if err := spinRun("Formatting root (ext4)", "mkfs.ext4", "-F", root); err != nil {
		return err
	}
	if homePart != "" {
		if err := spinRun("Formatting home (ext4)", "mkfs.ext4", "-F", homePart); err != nil {
			return err
		}
	}


	if err := spinRun("Mounting root", "mount", root, "/mnt"); err != nil {
		return err
	}
	if swapPart != "" {
		if err := spinRun("Enabling swap", "swapon", swapPart); err != nil {
			return err
		}
	}
	if err := os.MkdirAll("/mnt/boot/efi", 0755); err != nil {
		return err
	}
	if err := spinRun("Mounting EFI", "mount", efi, "/mnt/boot/efi"); err != nil {
		return err
	}
	if homePart != "" {
		if err := os.MkdirAll("/mnt/home", 0755); err != nil {
			return err
		}
		if err := spinRun("Mounting home", "mount", homePart, "/mnt/home"); err != nil {
			return err
		}
	}

	return nil
}


func installBase(cfg Config) error {
	pkgs := []string{
		"base", "base-devel", "linux-firmware",
		"networkmanager", "grub", "efibootmgr",
		"nano", "vim", "git", "fastfetch",
		cfg.Kernel, cfg.Kernel + "-headers",
	}

	switch {
	case strings.HasPrefix(cfg.GPU, "NVIDIA") && cfg.GPU != "NVIDIA 580xx (AUR, DKMS)":
		if cfg.Kernel == "linux" {
			pkgs = append(pkgs, "nvidia", "nvidia-utils", "nvidia-settings")
		} else {
			pkgs = append(pkgs, "nvidia-dkms", "nvidia-utils", "nvidia-settings")
		}
	case strings.HasPrefix(cfg.GPU, "Open Source"):
		pkgs = append(pkgs, "mesa", "vulkan-radeon", "vulkan-intel", "libva-mesa-driver")
	}

	switch cfg.Desktop {
	case "KDE Plasma":
		pkgs = append(pkgs, "plasma", "sddm", "konsole", "dolphin", "ark")
	case "XFCE4":
		pkgs = append(pkgs, "xfce4", "xfce4-goodies", "lightdm", "lightdm-gtk-greeter")
	case "Hyprland":
		pkgs = append(pkgs, "hyprland", "kitty", "waybar", "wofi", "xdg-desktop-portal-hyprland")
	}


	if cfg.Kernel == "linux-cachyos" {
		pkgs = append(pkgs, "cachyos-keyring", "cachyos-mirrorlist")
	}

	fmt.Println("[*] Running pacstrap — this may take a while...")
	return run("pacstrap", append([]string{"/mnt"}, pkgs...)...)
}


func configure(cfg Config) error {
	
	fstab, err := exec.Command("genfstab", "-U", "/mnt").Output()
	if err != nil {
		return fmt.Errorf("genfstab: %w", err)
	}
	if err := os.WriteFile("/mnt/etc/fstab", fstab, 0644); err != nil {
		return fmt.Errorf("writing fstab: %w", err)
	}

	
	logo := "\x1b[1;35m" +
		" ███████╗██╗  ██╗ ██████╗ ██████╗ ██╗████████╗███████╗\n" +
		" ██╔════╝╚██╗██╔╝██╔═══██╗██╔══██╗██║╚══██╔══╝██╔════╝\n" +
		" █████╗   ╚███╔╝ ██║   ██║██║  ██║██║   ██║   █████╗  \n" +
		" ██╔══╝   ██╔██╗ ██║   ██║██║  ██║██║   ██║   ██╔══╝  \n" +
		" ███████╗██╔╝ ██╗╚██████╔╝██████╔╝██║   ██║   ███████╗\n" +
		" ╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ╚══════╝" +
		"\x1b[0m"
	confJSON := `{"logo":{"source":"/etc/fastfetch/logo.txt"},"modules":["title","os","kernel","uptime","shell","de","cpu","memory"]}`

	if err := os.MkdirAll("/mnt/etc/fastfetch", 0755); err != nil {
		return err
	}
	if err := os.WriteFile("/mnt/etc/fastfetch/logo.txt", []byte(logo), 0644); err != nil {
		return err
	}
	if err := os.WriteFile("/mnt/etc/fastfetch/config.jsonc", []byte(confJSON), 0644); err != nil {
		return err
	}

	
	passwdLines := "root:" + cfg.RootPass + "\n" + cfg.Username + ":" + cfg.Password + "\n"
	if err := os.WriteFile("/mnt/passwd.tmp", []byte(passwdLines), 0600); err != nil {
		return fmt.Errorf("writing passwd temp file: %w", err)
	}

	script := buildChrootScript(cfg)
	if err := os.WriteFile("/mnt/setup.sh", []byte(script), 0700); err != nil {
		return fmt.Errorf("writing setup script: %w", err)
	}

	chrootErr := run("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")

	os.Remove("/mnt/setup.sh")
	os.Remove("/mnt/passwd.tmp")

	return chrootErr
}


func buildChrootScript(cfg Config) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }
	wf := func(f string, a ...any) { b.WriteString(fmt.Sprintf(f+"\n", a...)) }

	w("#!/bin/bash")
	w("set -e")
	w("")
	wf("GPU_DRIVER=%q", cfg.GPU)
	wf("INSTALL_YAY=%q", cfg.InstallYay)
	w("")

	wf("ln -sf /usr/share/zoneinfo/%s /etc/localtime", cfg.Timezone)
	w("hwclock --systohc")
	wf("echo '%s UTF-8' >> /etc/locale.gen", cfg.Locale)
	w("locale-gen")
	wf("echo 'LANG=%s' > /etc/locale.conf", cfg.Locale)
	wf("echo 'LC_ALL=%s' >> /etc/locale.conf", cfg.Locale)
	wf("echo '%s' > /etc/hostname", cfg.Hostname)
	wf("echo 'KEYMAP=%s' > /etc/vconsole.conf", cfg.Keymap)
	wf("printf '127.0.0.1 localhost\\n::1 localhost\\n127.0.1.1 %s.localdomain %s\\n' > /etc/hosts",
		cfg.Hostname, cfg.Hostname)
	w("")


	osRel := fmt.Sprintf("NAME=\"%s Linux\"\nID=%s\nID_LIKE=arch\nPRETTY_NAME=\"%s Linux\"\n",
		distroName, distroID, distroName)
	wf("printf '%%s' %q > /etc/os-release", osRel)
	w("")


	if cfg.Kernel == "linux-cachyos" {
		w("# ── CachyOS repo (chroot) ──")
		w("pacman-key --recv-keys F1656F40D7482129 || true")
		w("pacman-key --lsign-key F1656F40D7482129 || true")
		wf("grep -q '\\[cachyos\\]' /etc/pacman.conf || printf '%%s' %q >> /etc/pacman.conf", cachyRepoBlock)
		w("pacman -Sy --noconfirm")
		w("")
	}


	wf("sed -i 's/GRUB_DISTRIBUTOR=\"Arch\"/GRUB_DISTRIBUTOR=\"%s\"/' /etc/default/grub", distroName)
	if strings.HasPrefix(cfg.GPU, "NVIDIA") {
		w(`sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT="[^"]*/& nvidia-drm.modeset=1/' /etc/default/grub`)
	}
	w("")


	w(`echo "==> Fixing mkinitcpio hooks..."`)
	w(`sed -i 's/^HOOKS=.*/HOOKS=(base udev autodetect modconf block filesystems keyboard fsck)/' /etc/mkinitcpio.conf`)
	if strings.HasPrefix(cfg.GPU, "NVIDIA") {
		w(`sed -i 's/MODULES=()/MODULES=(nvidia nvidia_modeset nvidia_uvm nvidia_drm)/' /etc/mkinitcpio.conf`)
	}
	w(`echo "==> Building initramfs..."`)
	w("mkinitcpio -P")
	w("")


	w(`echo "==> Installing GRUB bootloader..."`)
	w(`ROOT_UUID=$(findmnt -n -o UUID /)`)
	w(`sed -i "s|^GRUB_CMDLINE_LINUX_DEFAULT=\"|GRUB_CMDLINE_LINUX_DEFAULT=\"root=UUID=$ROOT_UUID |" /etc/default/grub`)
	wf("grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=%s", distroName)
	w("grub-mkconfig -o /boot/grub/grub.cfg")
	w("")


	wf("useradd -m -G wheel -s /bin/bash '%s'", cfg.Username)
	w("pwconv")
	w("chpasswd < /passwd.tmp")
	w("rm -f /passwd.tmp")
	w("echo '%wheel ALL=(ALL:ALL) ALL' > /etc/sudoers.d/10-wheel")
	w("chmod 440 /etc/sudoers.d/10-wheel")
	w("")


	w("systemctl enable NetworkManager")
	switch cfg.Desktop {
	case "KDE Plasma":
		w("systemctl enable sddm")
		w("mkdir -p /etc/sddm.conf.d")
		wf("printf '[Autologin]\\nRelogin=false\\n\\n[Theme]\\nCurrent=breeze\\n' > /etc/sddm.conf.d/%s.conf",
			distroNameLower)
	case "XFCE4":
		w("systemctl enable lightdm")
	}
	w("")

	wf("echo 'fastfetch' >> /home/%s/.bashrc", cfg.Username)
	w("echo 'fastfetch' >> /root/.bashrc")
	w("")


	w(`# ── AUR helper ────────────────────────────────────────────────
build_yay() {
    useradd -m -s /bin/bash tempbuilder 2>/dev/null || true
    usermod -aG wheel tempbuilder
    echo '%wheel ALL=(ALL:ALL) NOPASSWD: ALL' > /etc/sudoers.d/99-yay-build
    chmod 440 /etc/sudoers.d/99-yay-build

    su - tempbuilder -c '
        export HOME=/home/tempbuilder
        cd "$HOME"
        git clone https://aur.archlinux.org/yay-bin.git
        cd yay-bin
        makepkg -si --noconfirm
        cd ..
        rm -rf yay-bin
    '

    rm -f /etc/sudoers.d/99-yay-build
    userdel -r tempbuilder 2>/dev/null || true
}

YAY_DONE=0
`)

	w(`if [ "$GPU_DRIVER" = "NVIDIA 580xx (AUR, DKMS)" ]; then
    echo "==> Installing yay for NVIDIA 580xx driver..."
    build_yay
    echo "==> Installing NVIDIA 580xx AUR driver..."
    yay -S --noconfirm nvidia-580xx-dkms nvidia-580xx-utils nvidia-580xx-settings
    YAY_DONE=1
fi
`)

	
	w(`if [ "$INSTALL_YAY" = "Yes" ] && [ "$YAY_DONE" -eq 0 ]; then
    echo "==> Installing yay..."
    build_yay
fi
`)

	return b.String()
}
