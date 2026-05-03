package internal

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func StartPortProxies(containerDir string, ports []string, targetIPCIDR string) ([]int, error) {
	targetIP := strings.Split(targetIPCIDR, "/")[0]
	if net.ParseIP(targetIP) == nil {
		return nil, fmt.Errorf("IP conteneur invalide: %s", targetIPCIDR)
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(containerDir, "ports.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()

	var pids []int
	for _, published := range ports {
		hostPort, containerPort, err := splitPublishedPort(published)
		if err != nil {
			CleanupPIDs(pids)
			return nil, err
		}
		bindAddr := strings.TrimSpace(os.Getenv("DOCKAN_PORT_BIND_ADDR"))
		if bindAddr == "" {
			bindAddr = "127.0.0.1"
		}
		if net.ParseIP(bindAddr) == nil {
			CleanupPIDs(pids)
			return nil, fmt.Errorf("adresse de publication invalide: %s", bindAddr)
		}
		listen := net.JoinHostPort(bindAddr, hostPort)
		target := net.JoinHostPort(targetIP, containerPort)
		cmd := exec.Command(exe, "__port-proxy", listen, target)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		cmd.Stdin = nil
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			CleanupPIDs(pids)
			return nil, fmt.Errorf("port %s: %w", published, err)
		}
		pids = append(pids, cmd.Process.Pid)
		_ = cmd.Process.Release()
	}
	return pids, nil
}

func RunPortProxy(listenAddr, targetAddr string) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()
	fmt.Printf("proxy %s -> %s\n", listenAddr, targetAddr)
	for {
		client, err := listener.Accept()
		if err != nil {
			return err
		}
		go proxyConnection(client, targetAddr)
	}
}

func proxyConnection(client net.Conn, targetAddr string) {
	defer client.Close()
	target, err := net.Dial("tcp", targetAddr)
	if err != nil {
		return
	}
	defer target.Close()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(target, client)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, target)
		done <- struct{}{}
	}()
	<-done
}

func CleanupPortProxies(meta map[string]string) error {
	return CleanupPIDs(splitPIDs(meta["portProxyPids"]))
}

func CleanupPIDs(pids []int) error {
	for _, pid := range pids {
		if pid <= 0 {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	return nil
}

func splitPublishedPort(published string) (string, string, error) {
	parts := strings.Split(published, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("port invalide: %s", published)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func splitPIDs(value string) []int {
	var pids []int
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pid, err := strconv.Atoi(part)
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
