package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

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

const (
	gpuNvidia    = "NVIDIA (proprietary)"
	gpuNvidia580 = "NVIDIA 580xx (AUR, DKMS)"
	gpuOpenSrc   = "Open Source (Intel / AMD / Nouveau)"
	gpuNone      = "None (No extra drivers)"
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println(red + "\n[!] Interrupted. Cleaning up..." + reset)
		cleanup()
		os.Exit(1)
	}()

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
	type step struct {
		name string
		fn   func(Config) error
	}

	steps := []step{
		{"Partitioning disk", partitionDisk},
		{"Installing base system", installBase},
		{"Configuring system", configure},
	}

	if cfg.Kernel == "linux-cachyos" {
		steps = []step{
			{"Partitioning disk", partitionDisk},
			{"Setting up CachyOS repository", func(c Config) error { return setupCachyLive() }},
			{"Installing base system", installBase},
			{"Configuring system", configure},
		}
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
	fmt.Printf("Disk:             %s\n", cfg.Disk)
	fmt.Printf("Partition layout: %s\n", cfg.PartLayout)
	if cfg.PartLayout == "split" || cfg.PartLayout == "dualboot" {
		fmt.Printf("  Root size:      %d GiB\n", cfg.RootSizeGB)
	}
	fmt.Printf("Kernel:           %s\n", cfg.Kernel)
	fmt.Printf("GPU driver:       %s\n", cfg.GPU)
	fmt.Printf("Desktop:          %s\n", cfg.Desktop)
	fmt.Printf("Yay (AUR helper): %s\n", cfg.InstallYay)
	fmt.Printf("Hostname:         %s\n", cfg.Hostname)
	fmt.Printf("Username:         %s\n", cfg.Username)
	fmt.Printf("Timezone:         %s\n", cfg.Timezone)
	fmt.Printf("Locale:           %s\n", cfg.Locale)
	fmt.Printf("Keymap:           %s\n", cfg.Keymap)

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
	fmt.Printf(purple+"  %s\n"+reset, title)
	fmt.Println()
	for i, opt := range options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}
	fmt.Println()

	for {
		answer := prompt(fmt.Sprintf("Select [1-%d]", len(options)), "", false)
		n, err := strconv.Atoi(answer)
		if err == nil && n >= 1 && n <= len(options) {
			return options[n-1]
		}
		fmt.Println(red + "  Invalid selection." + reset)
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
	keyReceived := false
	if err := spinner("Adding CachyOS key (ubuntu keyserver)", func() error {
		return exec.Command("pacman-key", "--recv-keys", "--keyserver", "hkps://keyserver.ubuntu.com", "F1656F40D7482129").Run()
	}); err == nil {
		keyReceived = true
	}
	if !keyReceived {
		if err := spinner("Adding CachyOS key (mailfence keyserver)", func() error {
			return exec.Command("pacman-key", "--recv-keys", "--keyserver", "hkps://keys.mailfence.com", "F1656F40D7482129").Run()
		}); err != nil {
			return fmt.Errorf("failed to receive CachyOS key from any keyserver: %w", err)
		}
	}

	if err := spinner("Signing CachyOS key", func() error {
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
		_, writeErr := f.WriteString("\n[cachyos]\nServer = https://mirror.cachyos.org/$repo/$arch\n")
		f.Close()
		if writeErr != nil {
			return writeErr
		}
	}

	return spinner("Syncing CachyOS keyring", func() error {
		return exec.Command("pacman", "-Sy", "--noconfirm", "cachyos-keyring").Run()
	})
}

var keymaps = []string{
	"us", "de", "uk", "fr", "es", "it", "pt", "ru", "pl", "nl", "colemak",
}

var locales = []string{
	"en_US.UTF-8", "en_GB.UTF-8", "de_DE.UTF-8", "fr_FR.UTF-8",
	"es_ES.UTF-8", "it_IT.UTF-8", "pt_PT.UTF-8", "ru_RU.UTF-8",
}

func validUsername(s string) bool {
	matched, _ := regexp.MatchString(`^[a-z_][a-z0-9_-]{0,31}$`, s)
	return matched
}

func validHostname(s string) bool {
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`, s)
	return matched
}

func validTimezone(s string) bool {
	_, err := os.Stat("/usr/share/zoneinfo/" + s)
	return err == nil
}

func gatherConfig() Config {
	var cfg Config

	cfg.Keymap = menuSelect("Keyboard Layout", keymaps)
	exec.Command("loadkeys", cfg.Keymap).Run()

	tz, _ := os.Readlink("/etc/localtime")
	defaultTZ := "UTC"
	if tz != "" && strings.HasPrefix(tz, "/usr/share/zoneinfo/") {
		defaultTZ = strings.TrimPrefix(tz, "/usr/share/zoneinfo/")
	}
	for {
		cfg.Timezone = prompt("Timezone", defaultTZ, false)
		if validTimezone(cfg.Timezone) {
			break
		}
		fmt.Println(red + "[!] Invalid timezone. Example: Europe/Berlin, America/New_York" + reset)
	}

	cfg.Locale = menuSelect("Locale", locales)

	out, _ := exec.Command("lsblk", "-d", "-n", "-o", "NAME,SIZE,MODEL").Output()
	type diskInfo struct {
		path      string
		size      string
		model     string
		sizeBytes uint64
	}
	var disksFound []diskInfo
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
			path:  "/dev/" + name,
			size:  fields[1],
			model: strings.Join(fields[2:], " "),
		}
		sizeOut, _ := exec.Command("lsblk", "-b", "-d", "-n", "-o", "SIZE", d.path).Output()
		if b, err := strconv.ParseUint(strings.TrimSpace(string(sizeOut)), 10, 64); err == nil && b > 0 {
			d.sizeBytes = b
		}
		disksFound = append(disksFound, d)
	}
	if len(disksFound) == 0 {
		fmt.Println(red + "[!] No disks found." + reset)
		os.Exit(1)
	}

	diskOptions := make([]string, len(disksFound))
	for i, d := range disksFound {
		diskOptions[i] = fmt.Sprintf("%s  (%s %s)", d.path, d.size, d.model)
	}
	diskChoice := menuSelect("Select Installation Disk", diskOptions)
	chosenPath := strings.Fields(diskChoice)[0]
	for _, d := range disksFound {
		if d.path == chosenPath {
			cfg.Disk = d.path
			cfg.DiskSizeBytes = d.sizeBytes
			break
		}
	}

	const minDiskBytes = 20 * 1024 * 1024 * 1024
	if cfg.DiskSizeBytes > 0 && cfg.DiskSizeBytes < minDiskBytes {
		fmt.Printf(yellow+"[!] Warning: disk is only %d GiB. Minimum recommended is 20 GiB.\n"+reset, cfg.DiskSizeBytes/1024/1024/1024)
		answer := prompt("Continue anyway? (yes/no)", "no", false)
		if strings.ToLower(answer) != "yes" && strings.ToLower(answer) != "y" {
			fmt.Println(cyan + "[*] Aborted." + reset)
			os.Exit(0)
		}
	}

	layout := menuSelect("Partition Layout", []string{
		"Single partition (root only)",
		"Separate /home partition",
		"Dualboot (alongside existing OS)",
	})
	switch layout {
	case "Separate /home partition":
		cfg.PartLayout = "split"
		defRoot := "30"
		if cfg.DiskSizeBytes > 0 {
			diskGB := cfg.DiskSizeBytes / 1024 / 1024 / 1024
			if diskGB < 40 {
				defRoot = fmt.Sprintf("%d", diskGB/2)
			}
		}
		for {
			sizeStr := prompt("Root partition size in GiB", defRoot, false)
			s, err := strconv.Atoi(sizeStr)
			if err == nil && s > 0 {
				cfg.RootSizeGB = s
				break
			}
			fmt.Println(red + "Invalid size, please enter a positive number." + reset)
		}
	case "Dualboot (alongside existing OS)":
		cfg.PartLayout = "dualboot"
		fmt.Println(yellow + "[!] Dualboot requires unallocated free space already on the disk." + reset)
		fmt.Println("    Use GParted or Windows Disk Management to shrink a partition first.")
		for {
			sizeStr := prompt("Root partition size in GiB", "30", false)
			s, err := strconv.Atoi(sizeStr)
			if err == nil && s > 0 {
				cfg.RootSizeGB = s
				break
			}
			fmt.Println(red + "Invalid size, please enter a positive number." + reset)
		}
	default:
		cfg.PartLayout = "single"
	}

	cfg.Kernel = menuSelect("Kernel", []string{"linux", "linux-lts", "linux-zen", "linux-cachyos"})

	cfg.GPU = menuSelect("Graphics Driver", []string{
		gpuNvidia,
		gpuNvidia580,
		gpuOpenSrc,
		gpuNone,
	})

	cfg.Desktop = menuSelect("Desktop Environment", []string{
		"KDE Plasma",
		"XFCE4",
		"Hyprland",
		"Sway",
		"i3",
		"Qtile",
		"AwesomeWM",
		"Cinnamon",
		"LXDE",
		"IceWM",
		"Niri",
		"None (TTY only)",
	})

	cfg.InstallYay = menuSelect("Install Yay (AUR helper)?", []string{"Yes", "No"})

	fmt.Println(purple + "\n--- User Accounts ---" + reset)

	for {
		cfg.Hostname = prompt("Hostname", distroNameLower, false)
		if validHostname(cfg.Hostname) {
			break
		}
		fmt.Println(red + "[!] Invalid hostname. Use letters, digits, and hyphens only." + reset)
	}

	for {
		cfg.Username = prompt("Username", "user", false)
		if validUsername(cfg.Username) {
			break
		}
		fmt.Println(red + "[!] Invalid username. Use lowercase letters, digits, hyphens, or underscores." + reset)
	}

	for {
		cfg.Password = prompt("User password", "", true)
		if cfg.Password == "" {
			fmt.Println(red + "[!] Password cannot be empty." + reset)
			continue
		}
		confirm := prompt("Confirm user password", "", true)
		if cfg.Password == confirm {
			break
		}
		fmt.Println(red + "[!] Passwords do not match." + reset)
	}

	for {
		cfg.RootPass = prompt("Root password", "", true)
		if cfg.RootPass == "" {
			fmt.Println(red + "[!] Password cannot be empty." + reset)
			continue
		}
		confirm := prompt("Confirm root password", "", true)
		if cfg.RootPass == confirm {
			break
		}
		fmt.Println(red + "[!] Passwords do not match." + reset)
	}

	return cfg
}

func checkFreeSpace(disk string) uint64 {
	out, err := exec.Command("sgdisk", "--print", disk).Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "free space") {
			continue
		}
		fields := strings.Fields(line)
		for i, f := range fields {
			if f != "free" || i+3 >= len(fields) {
				continue
			}
			if fields[i+1] != "space" {
				continue
			}
			raw := strings.TrimPrefix(fields[i+3], "(")
			switch {
			case strings.HasSuffix(raw, "KiB"):
				n, _ := strconv.ParseUint(strings.TrimSuffix(raw, "KiB"), 10, 64)
				return n * 1024
			case strings.HasSuffix(raw, "MiB"):
				n, _ := strconv.ParseUint(strings.TrimSuffix(raw, "MiB"), 10, 64)
				return n * 1024 * 1024
			case strings.HasSuffix(raw, "GiB"):
				n, _ := strconv.ParseUint(strings.TrimSuffix(raw, "GiB"), 10, 64)
				return n * 1024 * 1024 * 1024
			case strings.HasSuffix(raw, "TiB"):
				n, _ := strconv.ParseUint(strings.TrimSuffix(raw, "TiB"), 10, 64)
				return n * 1024 * 1024 * 1024 * 1024
			}
		}
	}
	return 0
}

func listPartitions(disk string) []string {
	var parts []string
	out, err := exec.Command("lsblk", "-nlo", "NAME", disk).Output()
	if err != nil {
		return parts
	}
	base := filepath.Base(disk)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		name = strings.TrimLeft(name, "├─└─ ")
		if name != "" && name != base {
			parts = append(parts, "/dev/"+name)
		}
	}
	return parts
}

func partitionDevice(disk string) string {
	if strings.Contains(disk, "nvme") || strings.Contains(disk, "mmcblk") {
		return disk + "p"
	}
	return disk
}

func partitionDisk(cfg Config) error {
	if cfg.PartLayout == "dualboot" {
		return partitionDiskDualboot(cfg)
	}

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

	exec.Command("udevadm", "settle", "--timeout=10").Run()

	p := partitionDevice(disk)
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

	if err := spinner("Mounting root", func() error {
		return exec.Command("mount", root, "/mnt").Run()
	}); err != nil {
		return err
	}
	os.MkdirAll("/mnt/boot/efi", 0755)
	if err := spinner("Mounting EFI", func() error {
		return exec.Command("mount", efi, "/mnt/boot/efi").Run()
	}); err != nil {
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
		if err := spinner("Mounting home", func() error {
			return exec.Command("mount", home, "/mnt/home").Run()
		}); err != nil {
			return err
		}
	}

	return nil
}

func partitionDiskDualboot(cfg Config) error {
	disk := cfg.Disk

	fmt.Println(cyan + "[*] Setting up dualboot — existing data will NOT be wiped" + reset)

	partsBefore := listPartitions(disk)

	var efiDevice string
	out, _ := exec.Command("lsblk", "-nlo", "NAME,PARTTYPE", disk).Output()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "c12a7328-f81f-11d2-ba4b-00a0c93ec93b" {
			efiDevice = "/dev/" + fields[0]
			break
		}
	}

	hasEFI := efiDevice != ""

	if hasEFI {
		fmt.Printf("[*] Reusing existing EFI partition: %s (will NOT format)\n", efiDevice)
	} else {
		fmt.Println("[*] No EFI partition found. Creating one in free space...")
		const efiNeedBytes = uint64(512) * 1024 * 1024
		freeBytes := checkFreeSpace(disk)
		if freeBytes > 0 && freeBytes < efiNeedBytes {
			return fmt.Errorf("not enough free space for EFI partition: need 512 MiB but only %d MiB available", freeBytes/1024/1024)
		}
		if err := spinner("Creating EFI partition (512 MiB)", func() error {
			out, err := exec.Command("sgdisk", "-n", "0:0:+512M", "-t", "0:ef00", disk).CombinedOutput()
			if err != nil {
				return fmt.Errorf("sgdisk: %s", strings.TrimSpace(string(out)))
			}
			return nil
		}); err != nil {
			return err
		}
		exec.Command("partprobe", disk).Run()
		exec.Command("udevadm", "settle", "--timeout=10").Run()

		partsAfter := listPartitions(disk)
		for _, p := range partsAfter {
			found := false
			for _, b := range partsBefore {
				if p == b {
					found = true
					break
				}
			}
			if !found {
				efiDevice = p
				break
			}
		}
		partsBefore = partsAfter
	}

	if efiDevice == "" {
		return fmt.Errorf("could not determine EFI partition")
	}

	needBytes := uint64(cfg.RootSizeGB) * 1024 * 1024 * 1024
	freeBytes := checkFreeSpace(disk)
	if freeBytes > 0 && freeBytes < needBytes {
		return fmt.Errorf("not enough free space: need %d GiB but only %d GiB available on %s",
			cfg.RootSizeGB, freeBytes/1024/1024/1024, disk)
	}

	if err := spinner(fmt.Sprintf("Creating root partition (%d GiB) in free space", cfg.RootSizeGB), func() error {
		out, err := exec.Command("sgdisk", "-n", "0:0:+"+strconv.Itoa(cfg.RootSizeGB)+"G", "-t", "0:8300", disk).CombinedOutput()
		if err != nil {
			return fmt.Errorf("sgdisk: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}); err != nil {
		return err
	}
	exec.Command("partprobe", disk).Run()
	exec.Command("udevadm", "settle", "--timeout=10").Run()

	var rootDevice string
	partsAfter := listPartitions(disk)
	for _, p := range partsAfter {
		found := false
		for _, b := range partsBefore {
			if p == b {
				found = true
				break
			}
		}
		if !found {
			rootDevice = p
		}
	}

	if rootDevice == "" {
		return fmt.Errorf("could not determine root partition")
	}

	if err := spinner("Formatting root (ext4)", func() error {
		return exec.Command("mkfs.ext4", "-F", rootDevice).Run()
	}); err != nil {
		return err
	}

	if !hasEFI {
		if err := spinner("Formatting EFI (FAT32)", func() error {
			return exec.Command("mkfs.fat", "-F32", efiDevice).Run()
		}); err != nil {
			return err
		}
	}

	if err := spinner("Mounting root", func() error {
		return exec.Command("mount", rootDevice, "/mnt").Run()
	}); err != nil {
		return err
	}
	os.MkdirAll("/mnt/boot/efi", 0755)
	if err := spinner("Mounting EFI", func() error {
		return exec.Command("mount", efiDevice, "/mnt/boot/efi").Run()
	}); err != nil {
		return err
	}

	fmt.Println(green + "[✓] Dualboot partitions ready." + reset)
	return nil
}

func installBase(cfg Config) error {
	pkgs := []string{
		"base", "base-devel", "linux-firmware",
		"networkmanager", "grub", "efibootmgr",
		"nano", "vim", "git", "fastfetch",
	}

	if cfg.Kernel != "linux-cachyos" {
		pkgs = append(pkgs, cfg.Kernel, cfg.Kernel+"-headers")
	}

	switch cfg.GPU {
	case gpuNvidia:
		if cfg.Kernel == "linux" || cfg.Kernel == "linux-lts" {
			pkgs = append(pkgs, "nvidia", "nvidia-utils", "nvidia-settings")
		} else {
			pkgs = append(pkgs, "nvidia-dkms", "nvidia-utils", "nvidia-settings")
		}
	case gpuOpenSrc:
		pkgs = append(pkgs, "mesa", "vulkan-radeon", "vulkan-intel", "libva-mesa-driver")
	}

	switch cfg.Desktop {
	case "KDE Plasma":
		pkgs = append(pkgs, "plasma", "sddm", "konsole", "dolphin", "ark")
	case "XFCE4":
		pkgs = append(pkgs, "xfce4", "xfce4-goodies", "lightdm", "lightdm-gtk-greeter")
	case "Hyprland":
		pkgs = append(pkgs, "hyprland", "kitty", "waybar", "wofi", "xdg-desktop-portal-hyprland", "sddm")
	case "Sway":
		pkgs = append(pkgs, "sway", "swaylock", "swayidle", "waybar", "wofi", "foot", "xdg-desktop-portal-wlr", "sddm")
	case "i3":
		pkgs = append(pkgs, "i3-wm", "i3status", "i3lock", "dmenu", "xorg-server", "xorg-xinit", "alacritty", "lightdm", "lightdm-gtk-greeter")
	case "Qtile":
		pkgs = append(pkgs, "qtile", "alacritty", "xorg-server", "xorg-xinit", "lightdm", "lightdm-gtk-greeter")
	case "AwesomeWM":
		pkgs = append(pkgs, "awesome", "alacritty", "xorg-server", "xorg-xinit", "lightdm", "lightdm-gtk-greeter")
	case "Cinnamon":
		pkgs = append(pkgs, "cinnamon", "gnome-terminal", "xorg-server", "lightdm", "lightdm-gtk-greeter")
	case "LXDE":
		pkgs = append(pkgs, "lxde", "lxdm", "xorg-server")
	case "IceWM":
		pkgs = append(pkgs, "icewm", "icewm-themes", "xorg-server", "xorg-xinit", "lightdm", "lightdm-gtk-greeter")
	case "Niri":
		pkgs = append(pkgs, "niri", "foot", "waybar", "wofi", "sddm")
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
	if err := os.WriteFile("/mnt/etc/fstab", fstab, 0644); err != nil {
		return err
	}

	logoANSI := purple +
		" ███████╗██╗  ██╗ ██████╗ ██████╗ ██╗████████╗███████╗\n" +
		" ██╔════╝╚██╗██╔╝██╔═══██╗██╔══██╗██║╚══██╔══╝██╔════╝\n" +
		" █████╗   ╚███╔╝ ██║   ██║██║  ██║██║   ██║   █████╗  \n" +
		" ██╔══╝   ██╔██╗ ██║   ██║██║  ██║██║   ██║   ██╔══╝  \n" +
		" ███████╗██╔╝ ██╗╚██████╔╝██████╔╝██║   ██║   ███████╗\n" +
		" ╚══════╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚═╝   ╚═╝   ╚══════╝" + reset

	confJSON := `{"logo":{"source":"/etc/fastfetch/logo.txt"},"modules":["title","os","kernel","uptime","shell","de","cpu","memory"]}`
	os.MkdirAll("/mnt/etc/fastfetch", 0755)
	os.WriteFile("/mnt/etc/fastfetch/logo.txt", []byte(logoANSI), 0644)
	os.WriteFile("/mnt/etc/fastfetch/config.jsonc", []byte(confJSON), 0644)

	osRel := fmt.Sprintf("NAME=\"%s Linux\"\nID=%s\nID_LIKE=arch\nPRETTY_NAME=\"%s Linux\"\n",
		distroName, distroID, distroName)

	script := "#!/bin/bash\nset -e\n"
	script += "trap 'rm -f /passwd.tmp' EXIT\n"
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

	script += fmt.Sprintf("printf '%%s' %q > /etc/os-release\n", osRel)
	script += fmt.Sprintf("sed -i 's/GRUB_DISTRIBUTOR=\"Arch\"/GRUB_DISTRIBUTOR=\"%s\"/' /etc/default/grub\n", distroName)

	if cfg.GPU == gpuNvidia || cfg.GPU == gpuNvidia580 {
		script += "sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT=\"[^\"]*/& nvidia-drm.modeset=1/' /etc/default/grub\n"
		script += "sed -i 's/MODULES=()/MODULES=(nvidia nvidia_modeset nvidia_uvm nvidia_drm)/' /etc/mkinitcpio.conf\n"
	}

	if cfg.Kernel == "linux-cachyos" {
		script += `
if ! grep -q '\[cachyos\]' /etc/pacman.conf; then
    printf '\n[cachyos]\nServer = https://mirror.cachyos.org/$repo/$arch\n' >> /etc/pacman.conf
fi
pacman-key --recv-keys --keyserver hkps://keyserver.ubuntu.com F1656F40D7482129 || \
    pacman-key --recv-keys --keyserver hkps://keys.mailfence.com F1656F40D7482129
pacman-key --lsign-key F1656F40D7482129
pacman -Sy --noconfirm cachyos-keyring cachyos-mirrorlist
pacman -S --noconfirm linux-cachyos linux-cachyos-headers
`
	}

	script += fmt.Sprintf("useradd -m -G wheel -s /bin/bash '%s'\n", cfg.Username)

	script += fmt.Sprintf("printf '%%s\\n%%s\\n' %q %q | passwd root\n", cfg.RootPass, cfg.RootPass)
	script += fmt.Sprintf("printf '%%s\\n%%s\\n' %q %q | passwd '%s'\n", cfg.Password, cfg.Password, cfg.Username)

	script += "echo '%wheel ALL=(ALL:ALL) ALL' > /etc/sudoers.d/10-wheel\n"
	script += "chmod 440 /etc/sudoers.d/10-wheel\n"

	script += `
YAY_DONE=0
if [ "$GPU_DRIVER" = "NVIDIA 580xx (AUR, DKMS)" ]; then
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
    yay -S --noconfirm nvidia-580xx-dkms nvidia-580xx-utils nvidia-580xx-settings
    YAY_DONE=1
fi
`

	script += `
sed -i 's/^HOOKS=.*/HOOKS=(base udev autodetect modconf block filesystems keyboard fsck)/' /etc/mkinitcpio.conf
mkinitcpio -P
`

	script += `
ROOT_UUID=$(findmnt -n -o UUID /)
sed -i "s|^GRUB_CMDLINE_LINUX_DEFAULT=\"|GRUB_CMDLINE_LINUX_DEFAULT=\"root=UUID=$ROOT_UUID |" /etc/default/grub
if [ -d /sys/firmware/efi ]; then
    grub-install --target=x86_64-efi --efi-directory=/boot/efi --bootloader-id=` + distroName + `
else
    DISK=$(lsblk -no PKNAME $(findmnt -n -o SOURCE /) | head -1)
    grub-install --target=i386-pc /dev/$DISK
fi
grub-mkconfig -o /boot/grub/grub.cfg
`

	script += "systemctl enable NetworkManager\n"

	switch cfg.Desktop {
	case "KDE Plasma", "Hyprland", "Sway", "Qtile", "Niri":
		script += "systemctl enable sddm\n"
	case "XFCE4", "i3", "AwesomeWM", "Cinnamon", "IceWM":
		script += "systemctl enable lightdm\n"
	case "LXDE":
		script += "systemctl enable lxdm\n"
	}

	if cfg.Desktop == "Niri" {
		script += "mkdir -p /etc/sddm.conf.d\n"
		script += "printf '[General]\\nDisplayServer=wayland\\nGreeterEnvironment=QT_WAYLAND_SHELL_INTEGRATION=layer-shell\\n' > /etc/sddm.conf.d/wayland.conf\n"
	}

	script += `
dd if=/dev/zero of=/swapfile bs=1M count=2048 status=none
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap defaults 0 0' >> /etc/fstab
`

	script += fmt.Sprintf("echo 'fastfetch' >> /home/%s/.bashrc\n", cfg.Username)
	script += "echo 'fastfetch' >> /root/.bashrc\n"

	script += `
if [ "$INSTALL_YAY" = "Yes" ] && [ "$YAY_DONE" -eq 0 ]; then
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
	defer os.Remove(scriptPath)

	fmt.Println(cyan + "[*] Running chroot configuration..." + reset)
	cmd := exec.Command("arch-chroot", "/mnt", "/bin/bash", "/setup.sh")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
