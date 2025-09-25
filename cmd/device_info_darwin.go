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
	"fmt"
	"net"
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
