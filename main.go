package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

type Config struct {
	Disk       string
	Kernel     string
	GPU        string
	Desktop    string
	Hostname   string
	Username   string
	Password   string
	RootPass   string
	Timezone   string
	Keymap     string
	Locale     string
	InstallYay string
}

func main() {
	if os.Getuid() != 0 {
		die("Root privileges required.")
	}

	setupNetwork()
	header("eXodite Linux Installer")

	cfg := gatherConfig()

	if err := runInstaller(cfg); err != nil {
		fmt.Printf("\n\033[31m[!] Installation failed: %v\033[0m\n", err)
		cleanup()
		os.Exit(1)
	}

	header("Success! Remove installation media and reboot.")
}

func runInstaller(cfg Config) error {
	if cfg.Kernel == "linux-cachyos" {
		if err := setupCachyLive(); err != nil {
			return fmt.Errorf("CachyOS setup: %w", err)
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
	fmt.Printf("\n\033[1;35m=== %s ===\033[0m\n\n", msg)
}

func die(msg string) {
	fmt.Printf("\033[31m[!] %s\033[0m\n", msg)
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
	fmt.Printf("\033[1;36m[*]\033[0m %s...", msg)
	if err := runSilent(cmd, args...); err != nil {
		fmt.Printf("\r\033[1;31m[✗]\033[0m %s... Failed!\n", msg)
		return err
	}
	fmt.Printf("\r\033[1;32m[✓]\033[0m %s\n", msg)
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
		fmt.Fprintf(tty, "\n\033[1;35m=== %s ===\033[0m\n\n", title)

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
				fmt.Fprintf(tty, "\033[1;32m   → %s\033[0m\n", options[i])
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
		fmt.Printf("\033[1;33m?\033[0m %s [%s]: ", msg, def)
	} else {
		fmt.Printf("\033[1;33m?\033[0m %s: ", msg)
	}

	if mask {
		fd := int(os.Stdin.Fd())
		b, err := term.ReadPassword(fd)
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
	fmt.Println("[*] Configuring CachyOS repository...")

	if err := spinRun("Receiving CachyOS key", "pacman-key", "--recv-keys", "F1656F40D7482129"); err != nil {
		return err
	}
	if err := spinRun("Signing CachyOS key", "pacman-key", "--lsign-key", "F1656F40D7482129"); err != nil {
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
		_, werr := f.WriteString("\n[cachyos]\nServer = https://mirror.cachyos.org/$repo/$arch\n")
		f.Close()
		if werr != nil {
			return fmt.Errorf("writing pacman.conf: %w", werr)
		}
	}
	return spinRun("Syncing CachyOS keyring", "pacman", "-Sy", "cachyos-keyring", "--noconfirm")
}

var timezones = []string{
	"Europe/Berlin",
	"Europe/London",
	"Europe/Paris",
	"Europe/Rome",
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"Asia/Tokyo",
	"Asia/Shanghai",
	"Asia/Kolkata",
	"Australia/Sydney",
	"UTC",
}

var keymaps = []string{
	"us",
	"de",
	"uk",
	"fr",
	"es",
	"it",
	"pt",
	"ru",
	"pl",
	"nl",
	"colemak",
}

var locales = []string{
	"en_US.UTF-8",
	"en_GB.UTF-8",
	"de_DE.UTF-8",
	"fr_FR.UTF-8",
	"es_ES.UTF-8",
	"it_IT.UTF-8",
	"pt_PT.UTF-8",
	"ru_RU.UTF-8",
}

func gatherConfig() Config {
	var cfg Config

	out, err := exec.Command("lsblk", "-d", "-n", "-o", "NAME,SIZE,MODEL").Output()
	if err != nil {
		die("Could not detect disks.")
	}

	var disks []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		name := strings.Fields(line)[0]
		if !strings.HasPrefix(name, "loop") {
			disks = append(disks, "/dev/"+name+" ("+strings.TrimSpace(line[len(name):])+")")
		}
	}
	if len(disks) == 0 {
		die("No target disks found.")
	}

	cfg.Keymap = strings.Fields(menuSelect("Keyboard Layout", keymaps))[0]
	runSilent("loadkeys", cfg.Keymap)

	cfg.Timezone = strings.Fields(menuSelect("Timezone", timezones))[0]
	cfg.Locale = strings.Fields(menuSelect("Locale", locales))[0]

	diskChoice := menuSelect("Target Disk (↑↓ to move, Enter to select)", disks)
	cfg.Disk = strings.Fields(diskChoice)[0]

	confirm := menuSelect(
		fmt.Sprintf("⚠ ALL DATA ON %s WILL BE DESTROYED — continue?", cfg.Disk),
		[]string{"No — go back", "Yes — wipe and install"},
	)
	if confirm != "Yes — wipe and install" {
		fmt.Println("Aborted.")
		os.Exit(0)
	}

	cfg.Kernel = menuSelect("Kernel", []string{"linux", "linux-zen", "linux-cachyos"})
	cfg.GPU = menuSelect("Graphics Driver", []string{
		"NVIDIA (proprietary)",
		"NVIDIA 580xx (AUR, DKMS)",
		"Open Source (Intel / AMD / Nouveau)",
		"None (No extra drivers)",
	})
	cfg.Desktop = menuSelect("Desktop Environment", []string{"KDE Plasma", "XFCE4", "Hyprland", "None (TTY only)"})
	cfg.InstallYay = menuSelect("Install Yay (AUR helper)?", []string{"Yes", "No"})

	fmt.Printf("\033[2J\033[H")
	header("User Accounts")

	cfg.Hostname = prompt("Hostname", "exodite", false)
	cfg.Username = prompt("Username", "user", false)

	for {
		cfg.Password = prompt("User password", "", true)
		confirm2 := prompt("Confirm user password", "", true)
		if cfg.Password == confirm2 {
			break
		}
		fmt.Println("\033[31m[!] Passwords do not match, try again.\033[0m")
	}

	for {
		cfg.RootPass = prompt("Root password", "", true)
		confirm2 := prompt("Confirm root password", "", true)
		if cfg.RootPass == confirm2 {
			break
		}
		fmt.Println("\033[31m[!] Passwords do not match, try again.\033[0m")
	}

	return cfg
}

func partition(cfg Config) error {
	disk := cfg.Disk

	if err := spinRun("Wiping partition table on "+disk, "sgdisk", "-Z", disk); err != nil {
		return err
	}
	if err := spinRun("Creating EFI partition (1 GiB)", "sgdisk", "-n", "1:0:+1G", "-t", "1:ef00", disk); err != nil {
		return err
	}
	if err := spinRun("Creating root partition (remainder)", "sgdisk", "-n", "2:0:0", "-t", "2:8300", disk); err != nil {
		return err
	}

	p := disk
	if strings.Contains(disk, "nvme") || strings.Contains(disk, "mmcblk") {
		p += "p"
	}
	efi := p + "1"
	root := p + "2"

	if err := spinRun("Formatting EFI (FAT32)", "mkfs.fat", "-F32", efi); err != nil {
		return err
	}
	if err := spinRun("Formatting root (ext4)", "mkfs.ext4", "-F", root); err != nil {
		return err
	}
	if err := spinRun("Mounting root", "mount", root, "/mnt"); err != nil {
		return err
	}
	if err := os.MkdirAll("/mnt/boot/efi", 0755); err != nil {
		return err
	}
	return spinRun("Mounting EFI", "mount", efi, "/mnt/boot/efi")
}

func installBase(cfg Config) error {
	pkgs := []string{
		"base", "sddm", "base-devel", "linux-firmware",
		"networkmanager", "grub", "efibootmgr",
		"nano", "vim", "git", "fastfetch",
		cfg.Kernel, cfg.Kernel + "-headers",
	}

	if strings.HasPrefix(cfg.GPU, "NVIDIA") && cfg.GPU != "NVIDIA 580xx (AUR, DKMS)" {
		if cfg.Kernel == "linux" {
			pkgs = append(pkgs, "nvidia", "nvidia-utils", "nvidia-settings")
		} else {
			pkgs = append(pkgs, "nvidia-dkms", "nvidia-utils", "nvidia-settings")
		}
	} else if strings.HasPrefix(cfg.GPU, "Open Source") {
		pkgs = append(pkgs, "mesa", "vulkan-radeon", "vulkan-intel", "libva-mesa-driver")
	}

	if cfg.Kernel == "linux-cachyos" {
		pkgs = append(pkgs, "cachyos-keyring", "cachyos-mirrorlist")
	}

	switch cfg.Desktop {
	case "KDE Plasma":
		pkgs = append(pkgs, "plasma", "sddm", "konsole", "dolphin", "ark")
	case "XFCE4":
		pkgs = append(pkgs, "xfce4", "xfce4-goodies", "lightdm", "lightdm-gtk-greeter")
	case "Hyprland":
		pkgs = append(pkgs, "hyprland", "kitty", "waybar", "wofi", "xdg-desktop-portal-hyprland")
	}

	fmt.Println("[*] Running pacstrap — this will take a while...")
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

	colorPurple := "\x1b[1;35m"
	colorReset := "\x1b[0m"
	logo := colorPurple +
		" ███████╗██╗  ██╗ ██████╗ ██████╗ ██╗████████╗███████╗\n" +
		" ██╔════╝╚██╗██╔╝██╔═══██╗██╔══██╗██║╚══██╔══╝██╔════╝\n" +
		" █████╗   ╚███╔╝ ██║   ██║██║  ██║██║   ██║   █████╗  \n" +
		" ██╔══╝   ██╔██╗ ██║   ██║██║  ██║██║   ██║   ██╔══╝  \n" +
		" ███████╗██╔╝ ██╗╚██████╔╝██████╔╝██║   ██║   ███████╗\n" +
		" ╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ╚══════╝" +
		colorReset
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
	passwdFile := "/mnt/passwd.tmp"
	if err := os.WriteFile(passwdFile, []byte(passwdLines), 0600); err != nil {
		return fmt.Errorf("writing passwd temp file: %w", err)
	}

	osRel := "NAME=\"eXodite Linux\"\nID=exodite\nID_LIKE=arch\nPRETTY_NAME=\"eXodite Linux\"\n"

	script := "#!/bin/bash\nset -e\n\n"
	script += fmt.Sprintf("GPU_DRIVER=%q\n", cfg.GPU)
	script += fmt.Sprintf("INSTALL_YAY=%q\n", cfg.InstallYay)

	script += fmt.Sprintf("ln -sf /usr/share/zoneinfo/%s /etc/localtime\n", cfg.Timezone)
	script += "hwclock --systohc\n"
	script += fmt.Sprintf("echo '%s UTF-8' >> /etc/locale.gen\n", cfg.Locale)
	script += "locale-gen\n"
	script += fmt.Sprintf("echo 'LANG=%s' > /etc/locale.conf\n", cfg.Locale)
	script += fmt.Sprintf("echo 'LC_ALL=%s' >> /etc/locale.conf\n", cfg.Locale)
	script += fmt.Sprintf("echo '%s' > /etc/hostname\n", cfg.Hostname)
	script += fmt.Sprintf("echo 'KEYMAP=%s' > /etc/vconsole.conf\n", cfg.Keymap)
	script += fmt.Sprintf("printf '127.0.0.1 localhost\\n::1 localhost\\n127.0.1.1 %s.localdomain %s\\n' > /etc/hosts\n",
		cfg.Hostname, cfg.Hostname)

	script += fmt.Sprintf("printf '%%s' %q | tee /etc/os-release > /dev/null\n", osRel)

	script += "sed -i 's/GRUB_DISTRIBUTOR=\"Arch\"/GRUB_DISTRIBUTOR=\"eXodite\"/' /etc/default/grub\n"

	if strings.HasPrefix(cfg.GPU, "NVIDIA") {
		script += "sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT=\"[^\"]*/& nvidia-drm.modeset=1/' /etc/default/grub\n"
		script += "sed -i 's/MODULES=()/MODULES=(nvidia nvidia_modeset nvidia_uvm nvidia_drm)/' /etc/mkinitcpio.conf\n"
	}

	if cfg.Kernel == "linux-cachyos" {
		script += `
if ! grep -q '\[cachyos\]' /etc/pacman.conf; then
    printf '\n[cachyos]\nServer = https://mirror.cachyos.org/$repo/$arch\n' >> /etc/pacman.conf
fi
pacman -Sy --noconfirm cachyos-keyring
`
	}

	script += fmt.Sprintf("useradd -m -G wheel -s /bin/bash '%s'\n", cfg.Username)
	script += "chpasswd < /passwd.tmp\n"
	script += "rm -f /passwd.tmp\n"
	script += "echo '%wheel ALL=(ALL:ALL) ALL' > /etc/sudoers.d/10-wheel\n"
	script += "chmod 440 /etc/sudoers.d/10-wheel\n"

	script += `
YAY_DONE=0

if [ "$GPU_DRIVER" = "NVIDIA 580xx (AUR, DKMS)" ]; then
    echo "==> Installing yay for NVIDIA 580xx driver..."
    useradd -m -s /bin/bash tempbuilder || true
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

    echo "==> Installing NVIDIA 580xx AUR driver..."
    yay -S --noconfirm nvidia-580xx-dkms nvidia-580xx-utils nvidia-580xx-settings

    YAY_DONE=1
fi
`

	script += `
echo "==> Fixing mkinitcpio hooks..."
sed -i 's/^HOOKS=.*/HOOKS=(base udev autodetect modconf block filesystems keyboard fsck)/' /etc/mkinitcpio.conf
`

	script += `
echo "==> Building initramfs..."
mkinitcpio -P

echo "==> Installing GRUB..."
ROOT_UUID=$(findmnt -n -o UUID /)
sed -i "s|^GRUB_CMDLINE_LINUX_DEFAULT=\"|GRUB_CMDLINE_LINUX_DEFAULT=\"root=UUID=$ROOT_UUID |" /etc/default/grub
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=eXodite
grub-mkconfig -o /boot/grub/grub.cfg
`

	script += "systemctl enable NetworkManager\n"
	switch cfg.Desktop {
	case "KDE Plasma":
		script += "systemctl enable sddm\n"
		script += "mkdir -p /etc/sddm.conf.d\n"
		script += "printf '[Autologin]\\nRelogin=false\\n\\n[Theme]\\nCurrent=breeze\\n' > /etc/sddm.conf.d/exodite.conf\n"
	case "XFCE4":
		script += "systemctl enable lightdm\n"
	}

	script += fmt.Sprintf("echo 'fastfetch' >> /home/%s/.bashrc\n", cfg.Username)
	script += "echo 'fastfetch' >> /root/.bashrc\n"

	script += `
if [ "$INSTALL_YAY" = "Yes" ] && [ "$YAY_DONE" -eq 0 ]; then
    echo "==> Installing yay for user..."
    useradd -m -s /bin/bash tempbuilder || true
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
fi
`

	scriptPath := "/mnt/setup.sh"
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return fmt.Errorf("writing setup script: %w", err)
	}

	err = run("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")

	os.Remove(scriptPath)
	os.Remove(passwdFile)

	return err
}
