package virtualboxapi

// TODO: all arguments to exec() not properly escaped

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/net"
)

type VMBootType string
type NetworkType string

const (
	SshPortRuleName = "terraform_ssh_port_rule"
)

const (
	Gui      VMBootType = "gui"
	Headless VMBootType = "headless"
	Sdl      VMBootType = "sdl"
	Separate VMBootType = "separate"
)

const (
	Bridged     NetworkType = "bridged"
	Nat         NetworkType = "nat"
	Hostonly    NetworkType = "nostonly"
	Hostonlynet NetworkType = "hostonlynet"
	Generic     NetworkType = "generic"
	Natnetwork  NetworkType = "natnetwork"
)

type VirtualboxVMInfo struct {
	ID       string
	Name     string
	State    string
	VmdkPath string
	SSHPort  string
}

func runGetOutput(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func CreateVM(imagePath, vmName string, memory, cpus int64) (*VirtualboxVMInfo, error) {
	cmd := exec.Command(
		"VBoxManage",
		"import",
		imagePath,
		"--vsys=0",
		fmt.Sprintf("--vmname=%s", vmName),
		fmt.Sprintf("--memory=%d", memory),
		fmt.Sprintf("--cpus=%d", cpus),
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	cmd = exec.Command(
		"VBoxManage",
		"modifyvm",
		vmName,
		"--nat-localhostreachable1",
		"on",
	)
	_, stderr, err = runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	return GetVMInfo(vmName)
}

func StartVM(vmName string, vmType VMBootType) (*VirtualboxVMInfo, error) {
	cmd := exec.Command(
		"VBoxManage",
		"startvm",
		vmName,
		fmt.Sprintf("--type=%s", vmType),
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	return GetVMInfo(vmName)
}

func StopVM(vmName string) (*VirtualboxVMInfo, error) {
	cmd := exec.Command(
		"VBoxManage",
		"controlvm",
		vmName,
		"poweroff",
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	return GetVMInfo(vmName)
}

func DestroyVM(vmName string) error {
	// VBoxManage unregistervm <uuid | vmname> [--delete] [--delete-all]
	cmd := exec.Command(
		"VBoxManage",
		"unregistervm",
		vmName,
		"--delete",
		"--delete-all",
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return errors.New(stderr)
	}
	return nil
}

func vmInfoValueToString(value string) string {
	if len(value) == 0 {
		return value
	}
	if value[0] == '"' {
		value = value[1:]
	}
	if len(value) > 0 && value[len(value)-1] == '"' {
		value = value[:len(value)-1]
	}
	return value
}

func GetVmIp(vminfo *VirtualboxVMInfo) (string, error) {
	cmd := exec.Command(
		"VBoxManage",
		"guestproperty",
		"enumerate",
		vminfo.ID,
		"/VirtualBox/GuestInfo/Net/0/V4/IP",
	)
	stdout, stderr, err := runGetOutput(cmd)
	if err != nil {
		return "", errors.New(stderr)
	}
	// example output:
	// /VirtualBox/GuestInfo/Net/0/V4/IP = '192.168.1.157' @ 2023-02-04T21:42:09.082Z
	ip := strings.Split(stdout, " ")[2]
	return ip[1 : len(ip)-1], nil
}

func GetVMInfo(vmName string) (*VirtualboxVMInfo, error) {
	cmd := exec.Command(
		"VBoxManage",
		"showvminfo",
		vmName,
		"--machinereadable",
	)
	stdout, stderr, err := runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	result := &VirtualboxVMInfo{}
	for _, line := range strings.Split(stdout, "\n") {
		keyValue := strings.SplitN(line, "=", 2)
		if len(keyValue) < 2 {
			// It's either a subkey without value or value with different format
			// https://docs.oracle.com/en/virtualization/virtualbox/6.0/user/vboxmanage-showvminfo.html
			continue
		}
		switch keyValue[0] {
		case "name":
			result.Name = vmInfoValueToString(keyValue[1])
		case "UUID":
			result.ID = vmInfoValueToString(keyValue[1])
		case "VMState":
			result.State = vmInfoValueToString(keyValue[1])
		case "\"SATA Controller-0-0\"":
			result.VmdkPath = vmInfoValueToString(keyValue[1])
		case "Forwarding(0)":
			splited := strings.Split(vmInfoValueToString(keyValue[1]), ",")
			result.SSHPort = splited[len(splited)-3]
		}
	}
	return result, nil
}

func ForwardLocalPort(vmName string, guestPort int) (*VirtualboxVMInfo, error) {
	ctx := context.Background()
	port, err := net.ListenRangeConfig{
		Addr:    "127.0.0.1",
		Min:     7000,
		Max:     8000,
		Network: "tcp",
	}.Listen(ctx)
	if err != nil {
		err := fmt.Errorf("Error creating port forwarding rule: %s", err)
		return nil, err
	}
	port.Listener.Close()

	// Make sure to configure the network interface to NAT
	cmd := exec.Command(
		"VBoxManage",
		"modifyvm",
		vmName,
		"--nic1",
		"nat",
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}

	// Create a forwarded port mapping to the VM
	cmd = exec.Command(
		"VBoxManage",
		"modifyvm",
		vmName,
		"--natpf1",
		fmt.Sprintf("%s,tcp,127.0.0.1,%d,,%d", SshPortRuleName, port.Port, guestPort),
	)
	_, stderr, err = runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	return GetVMInfo(vmName)
}

func InjectSSHKey(vmName, sshUser, sshKey string) error {
	vminfo, err := GetVMInfo(vmName)
	if err != nil {
		return err
	}
	// virt is not able to handle spaces in paths
	// virtualbox usually call vm dirs like "VirtualBox VMs"
	imageName := path.Base(vminfo.VmdkPath)
	tmpPath := path.Join("/tmp", imageName)

	input, err := os.Open(vminfo.VmdkPath)
	if err != nil {
		return err
	}
	defer input.Close()

	dst, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer dst.Close()
	defer os.Remove(tmpPath)

	_, err = io.Copy(dst, input)
	if err != nil {
		return err
	}
	err = dst.Sync()
	if err != nil {
		return err
	}

	cmd := exec.Command(
		"virt-sysprep",
		"-a",
		tmpPath,
		"--ssh-inject",
		fmt.Sprintf("%s:file:%s", sshUser, sshKey),
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return errors.New(stderr)
	}

	_, err = dst.Seek(0, 0)
	if err != nil {
		return err
	}
	output, err := os.Create(vminfo.VmdkPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, dst)
	if err != nil {
		return err
	}
	return nil
}
