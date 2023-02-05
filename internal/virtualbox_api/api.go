package virtualboxapi

// TODO: all arguments to exec() not properly escaped

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type VMBootType string
type NetworkType string

const (
	Gui      VMBootType = "gui"
	Headless            = "headless"
	Sdl                 = "sdl"
	Separate            = "separate"
)

const (
	Bridged     NetworkType = "bridged"
	Nat                     = "nat"
	Hostonly                = "nostonly"
	Hostonlynet             = "hostonlynet"
	Generic                 = "generic"
	Natnetwork              = "natnetwork"
)

type VirtualboxVMInfo struct {
	ID    string
	Name  string
	State string
	IPv4  string
}

type VirtualboxNicInfo struct {
	Type          string
	HostInterface string
}

func runGetOutput(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func ModifyNIC(vminfo *VirtualboxVMInfo, nic VirtualboxNicInfo) (*VirtualboxVMInfo, error) {
	cmd := exec.Command(
		"VBoxManage",
		"modifyvm",
		vminfo.ID,
		fmt.Sprintf("--nic1=%s", nic.Type),
		fmt.Sprintf("--bridge-adapter1=%s", nic.HostInterface), // TODO: this should be optional
	)
	_, stderr, err := runGetOutput(cmd)
	if err != nil {
		return nil, errors.New(stderr)
	}
	return GetVMInfo(vminfo.ID)
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
		}
	}
	if result.State == "running" {
		ip, err := GetVmIp(result)
		if err == nil {
			result.IPv4 = ip
		}
	}
	return result, nil
}
