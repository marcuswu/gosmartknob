package core

func CobsEncode(buf []byte) []byte {
	dest := make([]byte, 0, len(buf))
	codePtr := 0
	var code byte = 0x01

	finish := func(inclLast bool) {
		dest[codePtr] = code
		codePtr = len(dest)
		if inclLast {
			dest = append(dest, 0x00)
		}
		code = 0x01
	}

	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			finish(true)
		} else {
			dest = append(dest, buf[i])
			code += 1
			if code == 0xff {
				finish(true)
			}
		}
	}
	finish(false)

	return dest
}

func CobsDecode(buf []byte) []byte {
	dest := make([]byte, 0, len(buf))

	for i := 0; i < len(buf); i++ {
		code := buf[i]
		i++
		var j byte
		for j = 1; j < code; j++ {
			dest = append(dest, buf[i])
			i++
		}
		if code < 0xff && i < len(buf) {
			dest = append(dest, 0x00)
		}
	}

	return dest
}
