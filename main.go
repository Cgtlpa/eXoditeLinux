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

// Fixed run command so interactive tools like nmtui work perfectly!
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

// Interactive arrow-key menu
func menuSelect(title string, options []string) string {
	exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run()
	exec.Command("stty", "-F", "/dev/tty", "-echo").Run()

	selected := 0
	b := make([]byte, 3)

	for {
		fmt.Printf("\033[2J\033[H") // Clear screen
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

	exec.Command("stty", "-F", "/dev/tty", "sane").Run()
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
	
	// Force start NetworkManager just in case
	spinRun("Starting Network Manager", "systemctl", "start", "NetworkManager")

	// Check if we have internet by pinging Google's DNS
	err := exec.Command("ping", "-c", "1", "8.8.8.8").Run()
	if err != nil {
		fmt.Println("\n\033[33m[!] No internet connection detected.\033[0m")
		confirm := prompt("Would you like to configure Wi-Fi/Ethernet now? (y/n)")
		if strings.ToLower(confirm) == "y" {
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

	cfg.Disk = menuSelect("Select Target Disk (WILL BE WIPED)", disks)
	cfg.Kernel = menuSelect("Select Kernel", []string{"linux", "linux-zen", "linux-cachyos"})
	cfg.Desktop = menuSelect("Select Desktop Environment", []string{"KDE Plasma", "XFCE4", "Hyprland", "None"})

	fmt.Printf("\033[2J\033[H")
	header("System Configuration")
	cfg.Hostname = prompt("Enter Hostname")
	cfg.Username = prompt("Enter Username")
	cfg.Password = prompt("Enter User Password")
	cfg.RootPass = prompt("Enter Root Password")
	cfg.Timezone = "Europe/Berlin" 

	return cfg
}

func partition(cfg Config) {
	header("Partitioning Disk: " + cfg.Disk)
	spinRun("Wiping Disk", "sgdisk", "-Z", cfg.Disk)
	spinRun("Creating EFI Partition", "sgdisk", "-n", "1:0:+1G", "-t", "1:ef00", cfg.Disk)
	spinRun("Creating Root Partition", "sgdisk", "-n", "2:0:0", "-t", "2:8300", cfg.Disk)

	partPrefix := cfg.Disk
	if strings.Contains(cfg.Disk, "nvme") {
		partPrefix += "p"
	}

	spinRun("Formatting EFI", "mkfs.fat", "-F32", partPrefix+"1")
	spinRun("Formatting Root", "mkfs.ext4", "-F", partPrefix+"2")

	spinRun("Mounting Root", "mount", partPrefix+"2", "/mnt")
	runSilent("mkdir", "-p", "/mnt/boot/efi")
	spinRun("Mounting EFI", "mount", partPrefix+"1", "/mnt/boot/efi")
}

func installBase(cfg Config) {
	header("Installing Base System")
	packages := []string{"base", "base-devel", "linux-firmware", "networkmanager", "grub", "efibootmgr", "nano", "fastfetch", cfg.Kernel}

	if cfg.Desktop == "KDE Plasma" {
		packages = append(packages, "plasma", "sddm", "konsole")
	} else if cfg.Desktop == "XFCE4" {
		packages = append(packages, "xfce4", "xfce4-goodies", "lightdm", "lightdm-gtk-greeter")
	} else if cfg.Desktop == "Hyprland" {
		packages = append(packages, "hyprland", "kitty", "waybar")
	}

	args := append([]string{"/mnt"}, packages...)
	run("pacstrap", args...)
}

func configure(cfg Config) {
	header("Finalizing System")

	// Copy Fastfetch from Live USB to installed OS!
	runSilent("cp", "-r", "/etc/fastfetch", "/mnt/etc/")

	fstab, _ := exec.Command("genfstab", "-U", "/mnt").Output()
	os.WriteFile("/mnt/etc/fstab", fstab, 0644)

	osRelease := "NAME=\"eXodite Linux\"\nID=exodite\nID_LIKE=arch\nPRETTY_NAME=\"eXodite Linux\"\n"
	
	chrootScript := `#!/bin/bash
ln -sf /usr/share/zoneinfo/` + cfg.Timezone + ` /etc/localtime
hwclock --systohc
echo "en_US.UTF-8 UTF-8" >> /etc/locale.gen
locale-gen
echo "LANG=en_US.UTF-8" > /etc/locale.conf
echo "` + cfg.Hostname + `" > /etc/hostname
echo '` + osRelease + `' > /etc/os-release
echo "root:` + cfg.RootPass + `" | chpasswd
useradd -m -G wheel -s /bin/bash ` + cfg.Username + `
echo "` + cfg.Username + `:` + cfg.Password + `" | chpasswd
echo "%wheel ALL=(ALL:ALL) ALL" >> /etc/sudoers
mkinitcpio -P
grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=eXodite
grub-mkconfig -o /boot/grub/grub.cfg
systemctl enable NetworkManager
`

	if cfg.Desktop == "KDE Plasma" {
		chrootScript += "systemctl enable sddm\n"
	} else if cfg.Desktop == "XFCE4" {
		chrootScript += "systemctl enable lightdm\n"
	}

	chrootScript += `echo "fastfetch" >> /home/` + cfg.Username + `/.bashrc
echo "fastfetch" >> /root/.bashrc
`

	os.WriteFile("/mnt/setup.sh", []byte(chrootScript), 0755)
	run("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")
	os.Remove("/mnt/setup.sh")
}
