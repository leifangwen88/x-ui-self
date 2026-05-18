package service

import (
	"fmt"
	"strings"
)

func SocksNaturalKey(address string, port int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(address), port)
}

func ParseSocksNaturalKey(key string) (address string, port int, ok bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", 0, false
	}
	idx := strings.LastIndex(key, ":")
	if idx <= 0 {
		return "", 0, false
	}
	address = strings.TrimSpace(key[:idx])
	portStr := strings.TrimSpace(key[idx+1:])
	if address == "" || portStr == "" {
		return "", 0, false
	}
	var p int
	_, err := fmt.Sscanf(portStr, "%d", &p)
	if err != nil || p <= 0 || p > 65535 {
		return "", 0, false
	}
	return address, p, true
}
