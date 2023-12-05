package serial

import (
	"fmt"

	"github.com/marcuswu/gosmartknob/core"
	"github.com/rs/zerolog/log"
	"go.bug.st/serial/enumerator"
)

func FindPorts(filters []core.UsbFilter, serialNumber string, onlyUsb bool) []*enumerator.PortDetails {
	ports, err := enumerator.GetDetailedPortsList()
	filteredPorts := make([]*enumerator.PortDetails, 0, len(ports))

	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve serial port list")
		return []*enumerator.PortDetails{}
	}
	if len(ports) == 0 {
		log.Error().Msg("No serial ports found")
		return ports
	}

	for _, port := range ports {
		if !port.IsUSB {
			if onlyUsb {
				continue
			}
			filteredPorts = append(filteredPorts, port)
			continue
		}
		for _, filter := range filters {
			usbId := fmt.Sprintf("%s:%s", port.VID, port.PID)
			if usbId == filter.UsbId() &&
				(len(serialNumber) == 0 || serialNumber == port.SerialNumber) {
				filteredPorts = append(filteredPorts, port)
			}
		}
	}
	return filteredPorts
}
