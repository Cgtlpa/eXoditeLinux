package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Config struct {
	Disk     string
	Kernel   string
	Desktop  string
	Hostname string
	Username string
	Password string
	RootPass string
	Timezone string
}

func main() {
	if os.Getuid() != 0 {
		fmt.Println("Please run as root!")
		os.Exit(1)
	}

	setupNetwork()
	header("Welcome to the eXodite Linux Installer")
	cfg := gatherConfig()
	partition(cfg)
	installBase(cfg)
	configure(cfg)
	header("Installation Complete! You can now reboot.")
}

func header(msg string) {
	fmt.Printf("\n\033[36m=== %s ===\033[0m\n\n", msg)
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
	fmt.Printf("[ ] %s...", msg)
	err := runSilent(cmd, args...)
	if err != nil {
		fmt.Printf("\r[\033[31mX\033[0m] %s... Failed!\n", msg)
		os.Exit(1)
	}
	fmt.Printf("\r[\033[32m✓\033[0m] %s... Done!  \n", msg)
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
				fmt.Printf("\033[36m  → %s\033[0m\n", opt)
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

func prompt(msg string) string {
	fmt.Printf("%s: ", msg)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func setupNetwork() {
	header("Checking Network Connection")
	runSilent("systemctl", "start", "NetworkManager")
	err := exec.Command("ping", "-c", "1", "8.8.8.8").Run()
	if err != nil {
		fmt.Println("\n\033[33m[!] No internet connection detected.\033[0m")
		resp := prompt("Would you like to configure networking now? (y/n)")
		if strings.ToLower(resp) == "y" {
			run("nmtui")
		}
	} else {
		fmt.Println("[\033[32m✓\033[0m] Internet connection active!")
	}
}

func gatherConfig() Config {
	var cfg Config
	out, _ := exec.Command("lsblk", "-d", "-n", "-o", "NAME").Output()
	disks := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := range disks {
		disks[i] = "/dev/" + disks[i]
	}

	cfg.Disk = menuSelect("Select Target Disk", disks)
	cfg.Kernel = menuSelect("Select Kernel", []string{"linux", "linux-zen", "linux-cachyos"})
	cfg.Desktop = menuSelect("Select Desktop Environment", []string{"KDE Plasma", "XFCE4", "Hyprland", "None"})

	fmt.Printf("\033[2J\033[H")
	header("System Details")
	cfg.Hostname = prompt("Enter Hostname")
	cfg.Username = prompt("Enter Username")
	cfg.Password = prompt("Enter User Password")
	cfg.RootPass = prompt("Enter Root Password")
	cfg.Timezone = "Europe/Berlin"
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
	pkgs := []string{"base", "base-devel", "linux-firmware", "networkmanager", "grub", "efibootmgr", "nano", "fastfetch", cfg.Kernel}
	if cfg.Desktop == "KDE Plasma" {
		pkgs = append(pkgs, "plasma", "sddm", "konsole")
	} else if cfg.Desktop == "XFCE4" {
		pkgs = append(pkgs, "xfce4", "xfce4-goodies", "lightdm", "lightdm-gtk-greeter")
	} else if cfg.Desktop == "Hyprland" {
		pkgs = append(pkgs, "hyprland", "kitty", "waybar")
	}
	args := append([]string{"/mnt"}, pkgs...)
	run("pacstrap", args...)
}

func configure(cfg Config) {
	header("Finalizing Configuration")
	runSilent("cp", "-r", "/etc/fastfetch", "/mnt/etc/")
	fstab, _ := exec.Command("genfstab", "-U", "/mnt").Output()
	os.WriteFile("/mnt/etc/fstab", fstab, 0644)

	osRel := "NAME=\"eXodite Linux\"\nID=exodite\nID_LIKE=arch\nPRETTY_NAME=\"eXodite Linux\"\n"
	
	script := fmt.Sprintf(`#!/bin/bash
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
`, cfg.Timezone, cfg.Hostname, osRel, cfg.RootPass, cfg.Username, cfg.Username, cfg.Password)

	if cfg.Desktop == "KDE Plasma" { script += "systemctl enable sddm\n" }
	if cfg.Desktop == "XFCE4" { script += "systemctl enable lightdm\n" }
	script += fmt.Sprintf("echo \"fastfetch\" >> /home/%s/.bashrc\n", cfg.Username)
	script += "echo \"fastfetch\" >> /root/.bashrc\n"

	os.WriteFile("/mnt/setup.sh", []byte(script), 0755)
	run("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")
	os.Remove("/mnt/setup.sh")
}
