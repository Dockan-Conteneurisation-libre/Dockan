package internal

import (
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type NetworkOptions struct {
	Name    string
	Driver  string
	Subnet  string
	Gateway string
	Bridge  string
}

type NetworkAttachment struct {
	IP                 string
	HostInterface      string
	ContainerInterface string
}

func NetworksDir() string {
	return filepath.Join(StoreRoot(), "networks")
}

func CreateNetwork(name string) error {
	return CreateNetworkWithOptions(NetworkOptions{Name: name, Driver: "host-shared"})
}

func defaultComposeNetworkOptions(name string) NetworkOptions {
	hash := hashString(name)
	second := 64 + int(hash%64)
	third := int((hash / 64) % 256)
	return NetworkOptions{
		Name:    name,
		Driver:  "bridge",
		Subnet:  fmt.Sprintf("10.%d.%d.0/24", second, third),
		Gateway: fmt.Sprintf("10.%d.%d.1/24", second, third),
		Bridge:  defaultBridgeName(name),
	}
}

func defaultBridgeName(name string) string {
	bridge := "dockan-" + safeTag(name)
	if len(bridge) > 15 {
		bridge = bridge[:15]
	}
	return bridge
}

func CreateNetworkWithOptions(opts NetworkOptions) error {
	if opts.Driver == "" {
		opts.Driver = "host-shared"
	}
	if opts.Subnet == "" {
		opts.Subnet = "10.89.0.0/24"
	}
	if opts.Gateway == "" {
		opts.Gateway = "10.89.0.1/24"
	}
	if opts.Bridge == "" {
		opts.Bridge = defaultBridgeName(opts.Name)
	}
	name := opts.Name
	if err := validateNetworkName(name); err != nil {
		return err
	}
	if name == HostNetwork {
		return fmt.Errorf("le réseau %s est intégré", HostNetwork)
	}
	if opts.Driver != "host-shared" && opts.Driver != "bridge" {
		return fmt.Errorf("driver réseau invalide: %s", opts.Driver)
	}
	if opts.Driver == "bridge" {
		if _, _, err := net.ParseCIDR(opts.Subnet); err != nil {
			return fmt.Errorf("subnet invalide: %s", opts.Subnet)
		}
		if ip, _, err := net.ParseCIDR(opts.Gateway); err != nil || ip == nil {
			return fmt.Errorf("gateway invalide: %s", opts.Gateway)
		}
		if err := validateResourceName("nom de bridge", opts.Bridge); err != nil {
			return err
		}
	}
	path := filepath.Join(NetworksDir(), name+".conf")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("réseau déjà existant: %s", name)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return WriteMeta(path, map[string]string{
		"name":    name,
		"driver":  opts.Driver,
		"subnet":  opts.Subnet,
		"gateway": opts.Gateway,
		"bridge":  opts.Bridge,
		"created": time.Now().Format(time.RFC3339),
	})
}

func ListNetworks() error {
	fmt.Printf("%-20s %-12s %-18s %s\n", "NAME", "DRIVER", "SUBNET", "BRIDGE")
	fmt.Printf("%-20s %-12s %-18s %s\n", HostNetwork, "host-shared", "-", "-")
	entries, err := os.ReadDir(NetworksDir())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		meta, err := ParseMeta(filepath.Join(NetworksDir(), entry.Name()))
		if err != nil {
			continue
		}
		fmt.Printf("%-20s %-12s %-18s %s\n", meta["name"], meta["driver"], dash(meta["subnet"]), dash(meta["bridge"]))
	}
	return nil
}

func PrintNetworkDoctor() error {
	fmt.Println("Dockan network doctor")
	fmt.Printf("  ip:              %s\n", availability("ip"))
	fmt.Printf("  iptables:        %s\n", availability("iptables"))
	fmt.Printf("  sysctl:          %s\n", availability("sysctl"))
	fmt.Printf("  nsenter:         %s\n", availability("nsenter"))
	fmt.Printf("  utilisateur:     uid=%d\n", os.Geteuid())
	fmt.Printf("  ip_forward:      %s\n", readIPForward())
	fmt.Println()
	return ListNetworks()
}

func readIPForward() string {
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return "inconnu"
	}
	if strings.TrimSpace(string(data)) == "1" {
		return "ok"
	}
	return "désactivé"
}

func ListNetworkHosts(name string) error {
	if name != "" && name != HostNetwork {
		if _, err := loadNetwork(name); err != nil {
			return err
		}
	}
	fmt.Printf("%-20s %-20s %-18s %s\n", "NETWORK", "NAME", "IP", "IMAGE")
	containers, err := LoadContainers()
	if err != nil {
		return err
	}
	for _, container := range containers {
		if name != "" && container.Network != name {
			continue
		}
		meta, err := ParseMeta(filepath.Join(container.Path, "meta.conf"))
		if err != nil {
			continue
		}
		ip := strings.Split(meta["networkIP"], "/")[0]
		if ip == "" {
			ip = "-"
		}
		network := container.Network
		if network == "" {
			network = HostNetwork
		}
		fmt.Printf("%-20s %-20s %-18s %s\n", network, container.Name, ip, container.Image)
	}
	return nil
}

func WriteNetworkHosts(name string) error {
	if name == "" || name == HostNetwork {
		return nil
	}
	hosts, err := networkHosts(name)
	if err != nil {
		return err
	}
	containers, err := LoadContainers()
	if err != nil {
		return err
	}
	for _, container := range containers {
		if container.Network != name || container.Status != "running" {
			continue
		}
		meta, err := ParseMeta(filepath.Join(container.Path, "meta.conf"))
		if err != nil || meta["imagePath"] == "" {
			continue
		}
		if err := writeImageHosts(meta["imagePath"], hosts); err != nil {
			return err
		}
	}
	return nil
}

func RefreshNetwork(name string) error {
	if name == "" || name == HostNetwork {
		return fmt.Errorf("network refresh attend un réseau Dockan non-host")
	}
	if _, err := loadNetwork(name); err != nil {
		return err
	}
	if err := WriteNetworkHosts(name); err != nil {
		return err
	}
	fmt.Printf("Réseau rafraîchi: %s\n", name)
	return nil
}

func networkHosts(name string) (map[string]string, error) {
	hosts := map[string]string{}
	containers, err := LoadContainers()
	if err != nil {
		return hosts, err
	}
	for _, container := range containers {
		if container.Network != name || container.Status != "running" {
			continue
		}
		meta, err := ParseMeta(filepath.Join(container.Path, "meta.conf"))
		if err != nil {
			continue
		}
		ip := strings.Split(meta["networkIP"], "/")[0]
		if ip == "" {
			continue
		}
		hosts[container.Name] = ip
		for _, alias := range splitCSV(meta["aliases"]) {
			if alias != "" {
				hosts[alias] = ip
			}
		}
		if image := strings.Split(container.Image, ":")[0]; image != "" {
			hosts[image] = ip
		}
	}
	return hosts, nil
}

func writeImageHosts(imagePath string, hosts map[string]string) error {
	if len(hosts) == 0 {
		return nil
	}
	hostsPath := filepath.Join(imagePath, "rootfs", "etc", "hosts")
	if err := os.MkdirAll(filepath.Dir(hostsPath), 0755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("127.0.0.1 localhost\n")
	names := make([]string, 0, len(hosts))
	for name := range hosts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString(hosts[name])
		b.WriteByte(' ')
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return os.WriteFile(hostsPath, []byte(b.String()), 0644)
}

func RemoveNetwork(name string) error {
	if err := validateNetworkName(name); err != nil {
		return err
	}
	if name == HostNetwork {
		return fmt.Errorf("le réseau host intégré ne peut pas être supprimé")
	}
	path := filepath.Join(NetworksDir(), name+".conf")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("réseau introuvable: %s", name)
	}
	return os.Remove(path)
}

func EnableNetwork(name string) error {
	meta, err := loadNetwork(name)
	if err != nil {
		return err
	}
	if meta["driver"] != "bridge" {
		return fmt.Errorf("le réseau %s n'utilise pas le driver bridge", name)
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("bridge/NAT nécessite root: relancez avec sudo")
	}
	for _, command := range []string{"ip", "sysctl", "iptables"} {
		if _, err := exec.LookPath(command); err != nil {
			return fmt.Errorf("%s est requis pour activer bridge/NAT", command)
		}
	}
	bridge := meta["bridge"]
	gateway := meta["gateway"]
	subnet := meta["subnet"]
	if err := runNetworkCommand("ip", "link", "show", bridge); err != nil {
		if err := runNetworkCommand("ip", "link", "add", bridge, "type", "bridge"); err != nil {
			return err
		}
	}
	if err := runNetworkCommand("ip", "addr", "replace", gateway, "dev", bridge); err != nil {
		return err
	}
	if err := runNetworkCommand("ip", "link", "set", bridge, "up"); err != nil {
		return err
	}
	if err := runNetworkCommand("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return err
	}
	if err := runNetworkCommand("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", subnet, "!", "-o", bridge, "-j", "MASQUERADE"); err != nil {
		if err := runNetworkCommand("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", subnet, "!", "-o", bridge, "-j", "MASQUERADE"); err != nil && !strings.Contains(err.Error(), "exists") {
			return err
		}
	}
	fmt.Printf("Réseau bridge activé: %s (%s)\n", bridge, subnet)
	return nil
}

func DisableNetwork(name string) error {
	meta, err := loadNetwork(name)
	if err != nil {
		return err
	}
	if meta["driver"] != "bridge" {
		return fmt.Errorf("le réseau %s n'utilise pas le driver bridge", name)
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("bridge/NAT nécessite root: relancez avec sudo")
	}
	bridge := meta["bridge"]
	subnet := meta["subnet"]
	_ = runNetworkCommand("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", subnet, "!", "-o", bridge, "-j", "MASQUERADE")
	_ = runNetworkCommand("ip", "link", "delete", bridge)
	fmt.Printf("Réseau bridge désactivé: %s\n", bridge)
	return nil
}

func IsBridgeNetwork(name string) bool {
	if name == "" || name == HostNetwork {
		return false
	}
	meta, err := loadNetwork(name)
	return err == nil && meta["driver"] == "bridge"
}

func prepareDetachedNetwork(name string) (map[string]string, bool, error) {
	if name == "" || name == HostNetwork {
		return nil, false, nil
	}
	meta, err := loadNetwork(name)
	if err != nil {
		return nil, false, err
	}
	if meta["driver"] != "bridge" {
		return meta, false, nil
	}
	if os.Geteuid() != 0 {
		return nil, false, fmt.Errorf("réseau bridge nécessite root pour créer le namespace et la veth: relancez avec sudo")
	}
	for _, command := range []string{"ip", "nsenter"} {
		if _, err := exec.LookPath(command); err != nil {
			return nil, false, fmt.Errorf("%s est requis pour attacher un conteneur au bridge", command)
		}
	}
	return meta, true, nil
}

func AttachBridgeNetwork(containerName string, pid int, meta map[string]string) (NetworkAttachment, error) {
	bridge := meta["bridge"]
	networkName := meta["name"]
	if bridge == "" || networkName == "" {
		return NetworkAttachment{}, fmt.Errorf("métadonnées réseau bridge incomplètes")
	}
	ipCIDR, gateway, err := AllocateContainerAddress(networkName, containerName, meta["subnet"], meta["gateway"])
	if err != nil {
		return NetworkAttachment{}, err
	}
	hostIf := vethName("dh", containerName)
	containerIf := vethName("dc", containerName)
	_ = runNetworkCommand("ip", "link", "delete", hostIf)
	if err := runNetworkCommand("ip", "link", "add", hostIf, "type", "veth", "peer", "name", containerIf); err != nil {
		return NetworkAttachment{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = runNetworkCommand("ip", "link", "delete", hostIf)
		}
	}()
	if err := runNetworkCommand("ip", "link", "set", hostIf, "master", bridge); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("ip", "link", "set", hostIf, "up"); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("ip", "link", "set", containerIf, "netns", fmt.Sprint(pid)); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "link", "set", "lo", "up"); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "link", "set", containerIf, "name", "eth0"); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "addr", "add", ipCIDR, "dev", "eth0"); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "link", "set", "eth0", "up"); err != nil {
		return NetworkAttachment{}, err
	}
	if err := runNetworkCommand("nsenter", "-t", fmt.Sprint(pid), "-n", "--", "ip", "route", "replace", "default", "via", gateway); err != nil {
		return NetworkAttachment{}, err
	}
	cleanup = false
	return NetworkAttachment{IP: ipCIDR, HostInterface: hostIf, ContainerInterface: "eth0"}, nil
}

func CleanupContainerNetwork(meta map[string]string) error {
	if meta == nil || meta["vethHost"] == "" {
		return nil
	}
	if os.Geteuid() != 0 {
		return nil
	}
	_ = runNetworkCommand("ip", "link", "delete", meta["vethHost"])
	return nil
}

func AllocateContainerAddress(networkName, containerName, subnetCIDR, gatewayCIDR string) (string, string, error) {
	_, network, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return "", "", fmt.Errorf("subnet invalide: %s", subnetCIDR)
	}
	gatewayIP, _, err := net.ParseCIDR(gatewayCIDR)
	if err != nil {
		return "", "", fmt.Errorf("gateway invalide: %s", gatewayCIDR)
	}
	base := network.IP.To4()
	if base == nil || gatewayIP.To4() == nil {
		return "", "", fmt.Errorf("seuls les réseaux IPv4 sont supportés pour bridge")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones > 30 {
		return "", "", fmt.Errorf("subnet bridge trop petit: %s", subnetCIDR)
	}
	used := usedNetworkIPs(networkName)
	gateway := gatewayIP.To4().String()
	used[gateway] = true
	total := uint32(1) << uint32(32-ones)
	start := uint32(2 + (hashString(containerName) % maxUint32(1, total-3)))
	for offset := uint32(0); offset < total-2; offset++ {
		host := 1 + ((start + offset) % (total - 2))
		candidate := addIPv4(base, host)
		if candidate == nil || !network.Contains(candidate) {
			continue
		}
		ip := candidate.String()
		if !used[ip] {
			return ip + "/" + fmt.Sprint(ones), gateway, nil
		}
	}
	return "", "", fmt.Errorf("aucune IP libre dans %s", subnetCIDR)
}

func loadNetwork(name string) (map[string]string, error) {
	if err := validateNetworkName(name); err != nil {
		return nil, err
	}
	if name == HostNetwork {
		return map[string]string{"name": HostNetwork, "driver": "host-shared"}, nil
	}
	path := filepath.Join(NetworksDir(), name+".conf")
	meta, err := ParseMeta(path)
	if err != nil {
		return nil, fmt.Errorf("réseau introuvable: %s", name)
	}
	return meta, nil
}

func usedNetworkIPs(networkName string) map[string]bool {
	used := map[string]bool{}
	containers, err := LoadContainers()
	if err != nil {
		return used
	}
	for _, container := range containers {
		if container.Network != networkName {
			continue
		}
		meta, err := ParseMeta(filepath.Join(container.Path, "meta.conf"))
		if err != nil {
			continue
		}
		ip := strings.Split(meta["networkIP"], "/")[0]
		if ip != "" {
			used[ip] = true
		}
	}
	return used
}

func addIPv4(base net.IP, host uint32) net.IP {
	ip := append(net.IP(nil), base.To4()...)
	if ip == nil {
		return nil
	}
	value := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	value += host
	return net.IPv4(byte(value>>24), byte(value>>16), byte(value>>8), byte(value))
}

func hashString(value string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return h.Sum32()
}

func maxUint32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func vethName(prefix, containerName string) string {
	name := prefix + safeTag(containerName)
	if len(name) <= 15 {
		return name
	}
	hash := fmt.Sprintf("%x", hashString(containerName))
	maxBase := 15 - len(prefix) - 1 - len(hash[:6])
	if maxBase < 1 {
		maxBase = 1
	}
	base := safeTag(containerName)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return prefix + base + "-" + hash[:6]
}

func runNetworkCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
