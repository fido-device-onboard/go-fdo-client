// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

// #cgo LDFLAGS: -framework CoreFoundation -framework IOKit
// #include <CoreFoundation/CoreFoundation.h>
// #include <IOKit/IOKitLib.h>
//
// const char *
// getSerialNumber()
// {
//     CFMutableDictionaryRef matching = IOServiceMatching("IOPlatformExpertDevice");
//     io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, matching);
//     CFStringRef serialNumber = IORegistryEntryCreateCFProperty(service,
//         CFSTR("IOPlatformSerialNumber"), kCFAllocatorDefault, 0);
//     const char *str = CFStringGetCStringPtr(serialNumber, kCFStringEncodingUTF8);
//     IOObjectRelease(service);
//
//     return str;
// }
import "C"

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
)

func getSerial() (string, error) {
	serialNumber := C.GoString(C.getSerialNumber())
	return serialNumber, nil
}

func getMac(iface string) (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, i := range interfaces {
		if i.Name == iface {
			if i.HardwareAddr.String() == "00:00:00:00:00:00" {
				return "", fmt.Errorf("mac address for %s is zero", iface)
			}
			return i.HardwareAddr.String(), nil
		}
	}
	return "", fmt.Errorf("mac address for %s not found", iface)
}

func getSystemProfilerData(dataType string, v interface{}) error {
	cmd := exec.Command("system_profiler", dataType, "-json")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run system_profiler for %s: %w", dataType, err)
	}
	if err := json.Unmarshal(output, v); err != nil {
		return fmt.Errorf("failed to unmarshal json for %s: %w", dataType, err)
	}
	return nil
}

type SPSoftwareDataType struct {
	Name            string `json:"_name"`
	BootMode        string `json:"boot_mode"`
	BootVolume      string `json:"boot_volume"`
	KernelVersion   string `json:"kernel_version"`
	OSVersion       string `json:"os_version"`
	SecureVM        string `json:"secure_vm"`
	SystemIntegrity string `json:"system_integrity"`
	Uptime          string `json:"uptime"`
	UserName        string `json:"user_name"`
}

type SPSoftwareDataTypes struct {
	SPSSoftwareDataType []SPSoftwareDataType `json:"SPSoftwareDataType"`
}

func getOSVersion() (string, error) {
	var spSoftwareDataTypes SPSoftwareDataTypes
	err := getSystemProfilerData("SPSoftwareDataType", &spSoftwareDataTypes)
	if err != nil {
		return "", err
	}
	for _, spSoftwareDataType := range spSoftwareDataTypes.SPSSoftwareDataType {
		if spSoftwareDataType.Name == "os_overview" {
			return fmt.Sprintf("%s (%s)", spSoftwareDataType.OSVersion, spSoftwareDataType.KernelVersion), nil
		}
	}
	return "", fmt.Errorf("unable to determine the MacOS version")
}

type SPHardwareDataType struct {
	Name                 string `json:"_name"`
	ActivationLockStatus string `json:"activation_lock_status"`
	BootRomVersion       string `json:"boot_rom_version"`
	ChipType             string `json:"chip_type"`
	MachineModel         string `json:"machine_model"`
	MachineName          string `json:"machine_name"`
	ModelNumber          string `json:"model_number"`
	NumberProcessors     string `json:"number_processors"`
	OsLoaderVersion      string `json:"os_loader_version"`
	PhysicalMemory       string `json:"physical_memory"`
	PlatformUUID         string `json:"platform_UUID"`
	ProvisioningUDID     string `json:"provisioning_UDID"`
	SerialNumber         string `json:"serial_number"`
}

type SPHardwareDataTypes struct {
	SPHardwareDataType []SPHardwareDataType `json:"SPHardwareDataType"`
}

func getDeviceName() (string, error) {
	var spHardwareDataTypes SPHardwareDataTypes
	err := getSystemProfilerData("SPHardwareDataType", &spHardwareDataTypes)
	if err != nil {
		return "", err
	}
	for _, spHardwareDataType := range spHardwareDataTypes.SPHardwareDataType {
		if spHardwareDataType.Name == "hardware_overview" {
			return fmt.Sprintf("%s (%s)", spHardwareDataType.MachineName, spHardwareDataType.ChipType), nil
		}
	}
	return "", fmt.Errorf("unable to determine the device name")
}
