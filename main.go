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
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[1;32m"
	yellow = "\033[1;33m"
	cyan   = "\033[1;36m"
	purple = "\033[1;35m"
	white  = "\033[1;37m"
)

type Config struct {
	Disk          string
	DiskSizeBytes uint64
	PartLayout    string
	RootSizeGB    int
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
		fmt.Println(red + "[!] Root privileges required." + reset)
		os.Exit(1)
	}

	setupNetwork()
	printWelcome()

	cfg := gatherConfig()

	if !confirmInstall(cfg) {
		fmt.Println("\n" + cyan + "[*] Installation aborted by user." + reset)
		os.Exit(0)
	}

	if err := runInstaller(cfg); err != nil {
		fmt.Printf("\n"+red+"[!] Installation failed: %v"+reset+"\n", err)
		cleanup()
		os.Exit(1)
	}

	fmt.Println(green + "\n[✓] Installation complete! Remove the USB and reboot." + reset)
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
		{"Partitioning disk", partitionDisk},
		{"Installing base system", installBase},
		{"Configuring system", configure},
	}

	for _, s := range steps {
		fmt.Println(purple + "\n=== " + s.name + " ===" + reset)
		if err := s.fn(cfg); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

func cleanup() {
	fmt.Println("\n[*] Unmounting filesystems...")
	exec.Command("umount", "-R", "/mnt").Run()
}

func printWelcome() {
	logo := purple +
		" ███████╗██╗  ██╗ ██████╗ ██████╗ ██╗████████╗███████╗\n" +
		" ██╔════╝╚██╗██╔╝██╔═══██╗██╔══██╗██║╚══██╔══╝██╔════╝\n" +
		" █████╗   ╚███╔╝ ██║   ██║██║  ██║██║   ██║   █████╗  \n" +
		" ██╔══╝   ██╔██╗ ██║   ██║██║  ██║██║   ██║   ██╔══╝  \n" +
		" ███████╗██╔╝ ██╗╚██████╔╝██████╔╝██║   ██║   ███████╗\n" +
		" ╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ╚══════╝" + reset
	fmt.Println(logo)
	fmt.Println(white + "Welcome to the " + distroName + " Linux Installer!" + reset)
	fmt.Println("This wizard will guide you through the installation.\n")
}

func confirmInstall(cfg Config) bool {
	fmt.Println(purple + "\n=== Installation Summary ===" + reset)
	fmt.Printf("Disk:            %s\n", cfg.Disk)
	fmt.Printf("Partition layout: %s\n", cfg.PartLayout)
	if cfg.PartLayout == "split" {
		fmt.Printf("  Root size:     %d GiB\n", cfg.RootSizeGB)
	}
	fmt.Printf("Kernel:          %s\n", cfg.Kernel)
	fmt.Printf("GPU driver:      %s\n", cfg.GPU)
	fmt.Printf("Desktop:         %s\n", cfg.Desktop)
	fmt.Printf("Yay (AUR helper): %s\n", cfg.InstallYay)
	fmt.Printf("Hostname:        %s\n", cfg.Hostname)
	fmt.Printf("Username:        %s\n", cfg.Username)
	fmt.Printf("Timezone:        %s\n", cfg.Timezone)
	fmt.Printf("Locale:         %s\n", cfg.Locale)
	fmt.Printf("Keymap:         %s\n", cfg.Keymap)

	answer := prompt(yellow+"Proceed with installation? (yes/no)"+reset+" ", "no", false)
	return strings.ToLower(answer) == "yes" || strings.ToLower(answer) == "y"
}

func spinner(msg string, fn func() error) error {
	fmt.Print(cyan + "[*] " + msg + "... " + reset)
	err := fn()
	if err != nil {
		fmt.Print(red + "FAILED" + reset + "\n")
	} else {
		fmt.Print(green + "OK" + reset + "\n")
	}
	return err
}

func menuSelect(title string, options []string) string {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Println(red + "Cannot open /dev/tty: " + err.Error() + reset)
		os.Exit(1)
	}
	defer tty.Close()

	fd := int(tty.Fd())
	oldState, _ := term.MakeRaw(fd)
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
		fmt.Fprintf(tty, purple+"  %s\n"+reset, title)
		fmt.Fprintf(tty, white+"  Use ↑/↓ to move, Enter to select"+reset+"\n\n")

		if scrollOffset > 0 {
			fmt.Fprintf(tty, "     \033[90m↑ %d more\033[0m\n", scrollOffset)
		}

		end := scrollOffset + visibleRows
		if end > len(options) {
			end = len(options)
		}
		for i := scrollOffset; i < end; i++ {
			if i == selected {
				fmt.Fprintf(tty, green+"   → %s"+reset+"\n", options[i])
			} else {
				fmt.Fprintf(tty, "     %s\n", options[i])
			}
		}

		remaining := len(options) - end
		if remaining > 0 {
			fmt.Fprintf(tty, "     \033[90m↓ %d more\033[0m\n", remaining)
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
		fmt.Printf(yellow+"? "+reset+"%s ["+white+"%s"+reset+"]: ", msg, def)
	} else {
		fmt.Printf(yellow+"? "+reset+"%s: ", msg)
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

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func setupNetwork() {
	exec.Command("systemctl", "start", "NetworkManager").Run()
	if exec.Command("ping", "-c", "1", "-W", "3", "8.8.8.8").Run() != nil {
		fmt.Println(red + "[!] Network not available." + reset)
		if strings.ToLower(prompt("Open nmtui to connect? (Y/n)", "y", false)) == "y" {
			exec.Command("nmtui").Run()
		}
	}
}

func setupCachyLive() error {
	fmt.Println("[*] Configuring CachyOS repository...")
	if err := spinner("Adding CachyOS key", func() error {
		return exec.Command("pacman-key", "--recv-keys", "F1656F40D7482129").Run()
	}); err != nil {
		return err
	}
	if err := spinner("Signing key", func() error {
		return exec.Command("pacman-key", "--lsign-key", "F1656F40D7482129").Run()
	}); err != nil {
		return err
	}

	conf, err := os.ReadFile("/etc/pacman.conf")
	if err != nil {
		return err
	}
	if !strings.Contains(string(conf), "[cachyos]") {
		f, err := os.OpenFile("/etc/pacman.conf", os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, err = f.WriteString("\n[cachyos]\nServer = https://mirror.cachyos.org/$repo/$arch\n")
		f.Close()
		if err != nil {
			return err
		}
	}
	return spinner("Syncing CachyOS keyring", func() error {
		return exec.Command("pacman", "-Sy", "--noconfirm", "cachyos-keyring").Run()
	})
}

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

func gatherConfig() Config {
	var cfg Config

	cfg.Keymap = strings.Fields(menuSelect("Keyboard Layout", keymaps))[0]
	exec.Command("loadkeys", cfg.Keymap).Run()

	cfg.Timezone = strings.Fields(menuSelect("Timezone", timezones))[0]

	cfg.Locale = strings.Fields(menuSelect("Locale", locales))[0]

	out, _ := exec.Command("lsblk", "-d", "-n", "-o", "NAME,SIZE,MODEL").Output()
	type diskInfo struct {
		path     string
		size     string
		model    string
		sizeByte uint64
	}
	var disksInfo []diskInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		name := fields[0]
		if strings.HasPrefix(name, "loop") {
			continue
		}
		info := diskInfo{
			path:  "/dev/" + name,
			size:  fields[1],
			model: strings.Join(fields[2:], " "),
		}
		sizeBytes, _ := exec.Command("lsblk", "-b", "-d", "-n", "-o", "SIZE", info.path).Output()
		if b, _ := strconv.ParseUint(strings.TrimSpace(string(sizeBytes)), 10, 64); b > 0 {
			info.sizeByte = b
		}
		disksInfo = append(disksInfo, info)
	}
	if len(disksInfo) == 0 {
		fmt.Println(red + "[!] No disks found." + reset)
		os.Exit(1)
	}

	diskOptions := make([]string, len(disksInfo))
	for i, d := range disksInfo {
		diskOptions[i] = fmt.Sprintf("%s  (%s %s)", d.path, d.size, d.model)
	}
	diskChoice := menuSelect("Select Installation Disk", diskOptions)
	chosenPath := strings.Fields(diskChoice)[0]
	for _, d := range disksInfo {
		if d.path == chosenPath {
			cfg.Disk = d.path
			cfg.DiskSizeBytes = d.sizeByte
			break
		}
	}

	layout := menuSelect("Partition Layout", []string{
		"Single partition (root only)",
		"Separate /home partition",
	})
	if layout == "Separate /home partition" {
		cfg.PartLayout = "split"
		defRoot := "30"
		if cfg.DiskSizeBytes > 0 {
			diskGB := cfg.DiskSizeBytes / 1024 / 1024 / 1024
			if diskGB < 40 {
				defRoot = fmt.Sprintf("%d", diskGB/2)
			}
		}
		sizeStr := prompt("Root partition size in GiB", defRoot, false)
		s, err := strconv.Atoi(sizeStr)
		for err != nil || s <= 0 {
			fmt.Println(red + "Invalid size, please enter a positive number." + reset)
			sizeStr = prompt("Root partition size in GiB", defRoot, false)
			s, err = strconv.Atoi(sizeStr)
		}
		cfg.RootSizeGB = s
	} else {
		cfg.PartLayout = "single"
	}

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

	fmt.Println(purple + "\n--- User Accounts ---" + reset)
	cfg.Hostname = prompt("Hostname", distroNameLower, false)
	cfg.Username = prompt("Username", "user", false)

	for {
		cfg.Password = prompt("User password", "", true)
		confirm := prompt("Confirm user password", "", true)
		if cfg.Password == confirm {
			break
		}
		fmt.Println(red + "[!] Passwords do not match." + reset)
	}
	for {
		cfg.RootPass = prompt("Root password", "", true)
		confirm := prompt("Confirm root password", "", true)
		if cfg.RootPass == confirm {
			break
		}
		fmt.Println(red + "[!] Passwords do not match." + reset)
	}

	return cfg
}

func partitionDisk(cfg Config) error {
	disk := cfg.Disk

	if err := spinner("Wiping partition table", func() error {
		return exec.Command("sgdisk", "-Z", disk).Run()
	}); err != nil {
		return err
	}

	if cfg.PartLayout == "split" {
		if err := spinner("Creating EFI partition (1 GiB)", func() error {
			return exec.Command("sgdisk", "-n", "1:0:+1G", "-t", "1:ef00", disk).Run()
		}); err != nil {
			return err
		}
		if err := spinner(fmt.Sprintf("Creating root partition (%d GiB)", cfg.RootSizeGB), func() error {
			return exec.Command("sgdisk", "-n", "2:0:+"+strconv.Itoa(cfg.RootSizeGB)+"G", "-t", "2:8300", disk).Run()
		}); err != nil {
			return err
		}
		if err := spinner("Creating home partition (rest of disk)", func() error {
			return exec.Command("sgdisk", "-n", "3:0:0", "-t", "3:8300", disk).Run()
		}); err != nil {
			return err
		}
	} else {
		if err := spinner("Creating EFI partition (1 GiB)", func() error {
			return exec.Command("sgdisk", "-n", "1:0:+1G", "-t", "1:ef00", disk).Run()
		}); err != nil {
			return err
		}
		if err := spinner("Creating root partition (rest of disk)", func() error {
			return exec.Command("sgdisk", "-n", "2:0:0", "-t", "2:8300", disk).Run()
		}); err != nil {
			return err
		}
	}

	p := disk
	if strings.Contains(disk, "nvme") || strings.Contains(disk, "mmcblk") {
		p += "p"
	}
	efi := p + "1"
	root := p + "2"

	if err := spinner("Formatting EFI (FAT32)", func() error {
		return exec.Command("mkfs.fat", "-F32", efi).Run()
	}); err != nil {
		return err
	}
	if err := spinner("Formatting root (ext4)", func() error {
		return exec.Command("mkfs.ext4", "-F", root).Run()
	}); err != nil {
		return err
	}

	if err := spinner("Mounting root", func() error { return exec.Command("mount", root, "/mnt").Run() }); err != nil {
		return err
	}
	os.MkdirAll("/mnt/boot/efi", 0755)
	if err := spinner("Mounting EFI", func() error { return exec.Command("mount", efi, "/mnt/boot/efi").Run() }); err != nil {
		return err
	}

	if cfg.PartLayout == "split" {
		home := p + "3"
		if err := spinner("Formatting home (ext4)", func() error {
			return exec.Command("mkfs.ext4", "-F", home).Run()
		}); err != nil {
			return err
		}
		os.MkdirAll("/mnt/home", 0755)
		if err := spinner("Mounting home", func() error { return exec.Command("mount", home, "/mnt/home").Run() }); err != nil {
			return err
		}
	}
	return nil
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

	fmt.Println(cyan + "[*] Running pacstrap – this may take several minutes..." + reset)
	cmd := exec.Command("pacstrap", append([]string{"/mnt"}, pkgs...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func configure(cfg Config) error {
	fstab, err := exec.Command("genfstab", "-U", "/mnt").Output()
	if err != nil {
		return err
	}
	os.WriteFile("/mnt/etc/fstab", fstab, 0644)

	logo := purple +
		" ███████╗██╗  ██╗ ██████╗ ██████╗ ██╗████████╗███████╗\n" +
		" ██╔════╝╚██╗██╔╝██╔═══██╗██╔══██╗██║╚══██╔══╝██╔════╝\n" +
		" █████╗   ╚███╔╝ ██║   ██║██║  ██║██║   ██║   █████╗  \n" +
		" ██╔══╝   ██╔██╗ ██║   ██║██║  ██║██║   ██║   ██╔══╝  \n" +
		" ███████╗██╔╝ ██╗╚██████╔╝██████╔╝██║   ██║   ███████╗\n" +
		" ╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ╚══════╝" + reset

	confJSON := `{"logo":{"source":"/etc/fastfetch/logo.txt"},"modules":["title","os","kernel","uptime","shell","de","cpu","memory"]}`
	os.MkdirAll("/mnt/etc/fastfetch", 0755)
	os.WriteFile("/mnt/etc/fastfetch/logo.txt", []byte(logo), 0644)
	os.WriteFile("/mnt/etc/fastfetch/config.jsonc", []byte(confJSON), 0644)

	passwdLines := "root:" + cfg.RootPass + "\n" + cfg.Username + ":" + cfg.Password + "\n"
	os.WriteFile("/mnt/passwd.tmp", []byte(passwdLines), 0600)

	osRel := fmt.Sprintf("NAME=\"%s Linux\"\nID=%s\nID_LIKE=arch\nPRETTY_NAME=\"%s Linux\"\n",
		distroName, distroID, distroName)

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
	script += fmt.Sprintf("sed -i 's/GRUB_DISTRIBUTOR=\"Arch\"/GRUB_DISTRIBUTOR=\"%s\"/' /etc/default/grub\n", distroName)

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
echo "==> Building initramfs..."
sed -i 's/^HOOKS=.*/HOOKS=(base udev autodetect modconf block filesystems keyboard fsck)/' /etc/mkinitcpio.conf
mkinitcpio -P
`

	script += `
echo "==> Installing GRUB..."
ROOT_UUID=$(findmnt -n -o UUID /)
sed -i "s|^GRUB_CMDLINE_LINUX_DEFAULT=\"|GRUB_CMDLINE_LINUX_DEFAULT=\"root=UUID=$ROOT_UUID |" /etc/default/grub
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=` + distroName + `
grub-mkconfig -o /boot/grub/grub.cfg
`

	script += "systemctl enable NetworkManager\n"
	switch cfg.Desktop {
	case "KDE Plasma":
		script += "systemctl enable sddm\n"
	case "XFCE4":
		script += "systemctl enable lightdm\n"
	case "Hyprland":
		script += "systemctl enable sddm\n"
	}

	script += fmt.Sprintf("echo 'fastfetch' >> /home/%s/.bashrc\n", cfg.Username)
	script += "echo 'fastfetch' >> /root/.bashrc\n"

	script += `
if [ "$INSTALL_YAY" = "Yes" ] && [ "$YAY_DONE" -eq 0 ]; then
    echo "==> Installing yay..."
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
		return err
	}

	fmt.Println(cyan + "[*] Running chroot configuration..." + reset)
	cmd := exec.Command("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()

	os.Remove(scriptPath)
	os.Remove("/mnt/passwd.tmp")
	return err
}
