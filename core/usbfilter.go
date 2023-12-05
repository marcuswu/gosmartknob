package core

import "fmt"

type UsbFilter struct {
	VendorId  uint16
	ProductId uint16
}

func (f UsbFilter) UsbId() string {
	return fmt.Sprintf("%X:%X", f.VendorId, f.ProductId)
}
