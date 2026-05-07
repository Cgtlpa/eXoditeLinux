package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Config struct {
	Disk       string
	Kernel     string
	Desktop    string
	Hostname   string
	Username   string
	Password   string
	RootPass   string
	Timezone   string
	Keymap     string
	InstallYay string
}

func main() {
	if os.Getuid() != 0 {
		fmt.Println("\033[31m[!] Please run as root!\033[0m")
		os.Exit(1)
	}

	setupNetwork()
	header("Welcome to the eXodite Linux Installer")
	cfg := gatherConfig()
	
	if cfg.Kernel == "linux-cachyos" {
		setupCachyRepos()
	}

	partition(cfg)
	installBase(cfg)
	configure(cfg)
	header("Installation Complete! You can now reboot into eXodite Linux.")
}

func header(msg string) {
	fmt.Printf("\n\033[1;35m=== %s ===\033[0m\n\n", msg)
}

func run(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func runSilent(cmd string, args ...string) error {
	return exec.Command(cmd, args...).Run()
}

func spinRun(msg string, cmd string, args ...string) {
	fmt.Printf("\033[1;36m[*]\033[0m %s...", msg)
	err := runSilent(cmd, args...)
	if err != nil {
		fmt.Printf("\r\033[1;31m[X]\033[0m %s... Failed!\n", msg)
		os.Exit(1)
	}
	fmt.Printf("\r\033[1;32m[✓]\033[0m %s... Done!  \n", msg)
}

func menuSelect(title string, options []string) string {
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run()
	exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
	defer exec.Command("stty", "-F", "/dev/tty", "sane").Run()

	selected := 0
	b := make([]byte, 3)

	for {
		fmt.Printf("\033[2J\033[H")
		header(title)
		for i, opt := range options {
			if i == selected {
				fmt.Printf("\033[1;32m  → %s\033[0m\n", opt)
			} else {
				fmt.Printf("    %s\n", opt)
			}
		}

		os.Stdin.Read(b)
		if b[0] == 27 && b[1] == 91 {
			if b[2] == 65 && selected > 0 { selected-- }
			if b[2] == 66 && selected < len(options)-1 { selected++ }
		} else if b[0] == 10 {
			break
		}
	}
	return options[selected]
}

func prompt(msg, def string) string {
	if def != "" {
		fmt.Printf("\033[1;33m?\033[0m %s [\033[1;36m%s\033[0m]: ", msg, def)
	} else {
		fmt.Printf("\033[1;33m?\033[0m %s: ", msg)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" { return def }
	return val
}

func setupNetwork() {
	header("Network Check")
	runSilent("systemctl", "start", "NetworkManager")
	err := exec.Command("ping", "-c", "1", "8.8.8.8").Run()
	if err != nil {
		fmt.Println("\033[31m[!] No internet detected.\033[0m")
		resp := prompt("Configure networking now? (y/n)", "y")
		if strings.ToLower(resp) == "y" {
			run("nmtui")
		}
	} else {
		fmt.Println("\033[1;32m[✓]\033[0m Internet active!")
	}
}

func setupCachyRepos() {
	header("Preparing CachyOS Repos")
	runSilent("pacman-key", "--init")
	runSilent("pacman-key", "--populate", "archlinux")
	run("pacman-key", "--recv-keys", "F1656F40D7482129")
	run("pacman-key", "--lsign-key", "F1656F40D7482129")
	runSilent("sh", "-c", "echo 'Server = https://mirror.cachyos.org/repo/$arch/$repo' > /etc/pacman.d/cachyos-mirrorlist")
	runSilent("sh", "-c", "grep -q 'cachyos' /etc/pacman.conf || echo -e '\n[cachyos]\nInclude = /etc/pacman.d/cachyos-mirrorlist' >> /etc/pacman.conf")
	run("pacman", "-Sy", "--noconfirm")
}

func gatherConfig() Config {
	var cfg Config
	out, _ := exec.Command("lsblk", "-d", "-n", "-o", "NAME").Output()
	rawDisks := strings.Split(strings.TrimSpace(string(out)), "\n")
	var disks []string
	for _, d := range rawDisks {
		if !strings.HasPrefix(d, "loop") && d != "" { disks = append(disks, "/dev/"+d) }
	}

	cfg.Keymap = menuSelect("Keyboard Layout", []string{"us", "de", "uk", "fr", "es"})
	runSilent("loadkeys", cfg.Keymap)
	cfg.Timezone = menuSelect("Select Timezone", []string{"Europe/Berlin", "Europe/London", "America/New_York", "UTC"})
	cfg.Disk = menuSelect("Target Disk", disks)
	cfg.Kernel = menuSelect("Select Kernel", []string{"linux", "linux-zen", "linux-cachyos"})
	cfg.Desktop = menuSelect("Desktop Environment", []string{"KDE Plasma", "XFCE4", "Hyprland", "None"})
	cfg.InstallYay = menuSelect("Install Yay (AUR Helper)?", []string{"Yes", "No"})

	fmt.Printf("\033[2J\033[H")
	header("User Setup")
	cfg.Hostname = prompt("Hostname", "exodite")
	cfg.Username = prompt("Username", "adrian")
	cfg.Password = prompt("User Password", "")
	cfg.RootPass = prompt("Root Password", "")
	return cfg
}

func partition(cfg Config) {
	header("Partitioning " + cfg.Disk)
	spinRun("Wiping disk", "sgdisk", "-Z", cfg.Disk)
	spinRun("Creating EFI", "sgdisk", "-n", "1:0:+1G", "-t", "1:ef00", cfg.Disk)
	spinRun("Creating Root", "sgdisk", "-n", "2:0:0", "-t", "2:8300", cfg.Disk)
	prefix := cfg.Disk
	if strings.Contains(cfg.Disk, "nvme") { prefix += "p" }
	spinRun("Formatting EFI", "mkfs.fat", "-F32", prefix+"1")
	spinRun("Formatting Root", "mkfs.ext4", "-F", prefix+"2")
	spinRun("Mounting Root", "mount", prefix+"2", "/mnt")
	os.MkdirAll("/mnt/boot/efi", 0755)
	spinRun("Mounting EFI", "mount", prefix+"1", "/mnt/boot/efi")
}

func installBase(cfg Config) {
	header("Installing eXodite Base")
	pkgs := []string{"base", "base-devel", "linux-firmware", "networkmanager", "grub", "efibootmgr", "nano", "git", "fastfetch", cfg.Kernel}
	if cfg.Kernel == "linux-cachyos" {
		pkgs = append(pkgs, "cachyos-keyring", "cachyos-mirrorlist", "cachyos-hooks")
	}
	switch cfg.Desktop {
	case "KDE Plasma": pkgs = append(pkgs, "plasma", "sddm", "konsole")
	case "XFCE4": pkgs = append(pkgs, "xfce4", "xfce4-goodies", "lightdm", "lightdm-gtk-greeter")
	case "Hyprland": pkgs = append(pkgs, "hyprland", "kitty", "waybar")
	}
	args := append([]string{"/mnt"}, pkgs...)
	err := run("pacstrap", args...)
	if err != nil { os.Exit(1) }
}

func configure(cfg Config) {
	header("Finalizing Configuration")
	fstab, _ := exec.Command("genfstab", "-U", "/mnt").Output()
	os.WriteFile("/mnt/etc/fstab", fstab, 0644)

	osRel := "NAME=\"eXodite Linux\"\nID=exodite\nID_LIKE=arch\nPRETTY_NAME=\"eXodite Linux\"\n"
	
	// EMBEDDED FASTFETCH CONFIG (Using hex for color to avoid escape errors)
	colorPurple := "\x1b[1;35m"
	colorReset := "\x1b[0m"
	logo := colorPurple + ` ███████╗██╗  ██╗ ██████╗ ██████╗ ██╗████████╗███████╗
 ██╔════╝╚██╗██╔╝██╔═══██╗██╔══██╗██║╚══██╔══╝██╔════╝
 █████╗   ╚███╔╝ ██║   ██║██║  ██║██║   ██║   █████╗  
 ██╔══╝   ██╔██╗ ██║   ██║██║  ██║██║   ██║   ██╔══╝  
 ███████╗██╔╝ ██╗╚██████╔╝██████╔╝██║   ██║   ███████╗
 ╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ╚══════╝` + colorReset

	confJson := `{"logo": {"source": "/etc/fastfetch/logo.txt"}, "modules": ["title","os","kernel","uptime","packages","shell","de","wm","cpu","gpu","memory"]}`

	os.MkdirAll("/mnt/etc/fastfetch", 0755)
	os.WriteFile("/mnt/etc/fastfetch/logo.txt", []byte(logo), 0644)
	os.WriteFile("/mnt/etc/fastfetch/config.jsonc", []byte(confJson), 0644)

	script := fmt.Sprintf(`#!/bin/bash
echo "KEYMAP=%s" > /etc/vconsole.conf
ln -sf /usr/share/zoneinfo/%s /etc/localtime
hwclock --systohc
echo "en_US.UTF-8 UTF-8" >> /etc/locale.gen
locale-gen
echo "LANG=en_US.UTF-8" > /etc/locale.conf
echo "%s" > /etc/hostname
echo '%s' > /etc/os-release
sed -i 's/GRUB_DISTRIBUTOR="Arch"/GRUB_DISTRIBUTOR="eXodite"/' /etc/default/grub
echo "root:%s" | chpasswd
useradd -m -G wheel -s /bin/bash %s
echo "%s:%s" | chpasswd
echo "%%wheel ALL=(ALL:ALL) ALL" >> /etc/sudoers
mkinitcpio -P
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=eXodite
grub-mkconfig -o /boot/grub/grub.cfg
systemctl enable NetworkManager
`, cfg.Keymap, cfg.Timezone, cfg.Hostname, osRel, cfg.RootPass, cfg.Username, cfg.Username, cfg.Password)

	if cfg.Desktop == "KDE Plasma" { script += "systemctl enable sddm\n" }
	if cfg.Desktop == "XFCE4" { script += "systemctl enable lightdm\n" }
	script += fmt.Sprintf("echo \"fastfetch\" >> /home/%s/.bashrc\n", cfg.Username)
	script += "echo \"fastfetch\" >> /root/.bashrc\n"

	if cfg.InstallYay == "Yes" {
		script += fmt.Sprintf("su - %s -c \"git clone https://aur.archlinux.org/yay-bin.git ~/yay-bin && cd ~/yay-bin && makepkg -si --noconfirm\"\n", cfg.Username)
	}

	os.WriteFile("/mnt/setup.sh", []byte(script), 0755)
	os.Chmod("/mnt/setup.sh", 0755)
	run("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")
	os.Remove("/mnt/setup.sh")
}
