package internal

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/yl2chen/cidranger"
)

var IPV4_FULL_MASK = net.CIDRMask(32, 32)
var IPV6_FULL_MASK = net.CIDRMask(128, 128)

type IPBans struct {
	Blacklist cidranger.Ranger
	Whitelist cidranger.Ranger
}

func (ipBans *IPBans) IsBanned(ip net.IP) (bool, error) {
	banned, err := ipBans.Blacklist.Contains(ip)
	if err != nil {
		return false, err
	}

	if !banned {
		return false, nil
	}

	// IP is in the black list, check white list

	allowed, err := ipBans.Whitelist.Contains(ip)
	if err != nil {
		return true, err
	}

	return !allowed, nil
}

func parseLine(line string) (*net.IPNet, error) {
	_, ipnet, err := net.ParseCIDR(line)
	if err == nil {
		return ipnet, nil
	}

	ip := net.ParseIP(line)
	if ip == nil {
		return nil, fmt.Errorf("Failed to parse IP or IP subnet \"%s\"", line)
	}

	ipv4 := ip.To4()
	if ipv4 != nil {
		return &net.IPNet{
			IP:   ipv4,
			Mask: IPV4_FULL_MASK,
		}, nil
	}

	ipv6 := ip.To16()
	if ipv6 != nil {
		return &net.IPNet{
			IP:   ipv6,
			Mask: IPV6_FULL_MASK,
		}, nil
	}

	return nil, fmt.Errorf("Unknown IP address format: %v", ip)
}

func parseIPList(reader io.Reader) (cidranger.Ranger, error) {
	scanner := bufio.NewScanner(reader)
	ranger := cidranger.NewPCTrieRanger()

	for scanner.Scan() {
		line := scanner.Text()

		// Remove comments
		commentIdx := strings.IndexByte(line, '#')
		if commentIdx >= 0 {
			line = line[:commentIdx]
		}

		line = strings.TrimSpace(line)

		if len(line) == 0 {
			continue
		}

		ipnet, err := parseLine(line)
		if err != nil {
			return nil, err
		}

		err = ranger.Insert(cidranger.NewBasicRangerEntry(*ipnet))
		if err != nil {
			return nil, err
		}
	}

	err := scanner.Err()
	if err != nil {
		return nil, err
	}

	return ranger, nil
}

func parseIPListFile(path string) (cidranger.Ranger, error) {
	if len(path) == 0 {
		return cidranger.NewPCTrieRanger(), nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseIPList(file)
}

func LoadIPBans(blacklistPath string, whitelistPath string) (IPBans, error) {
	var ipBans IPBans
	var err error

	ipBans.Blacklist, err = parseIPListFile(blacklistPath)
	if err != nil {
		return ipBans, err
	}

	ipBans.Whitelist, err = parseIPListFile(whitelistPath)
	if err != nil {
		return ipBans, err
	}

	return ipBans, nil
}

func EmptyIPBans() IPBans {
	return IPBans{
		Blacklist: cidranger.NewPCTrieRanger(),
		Whitelist: cidranger.NewPCTrieRanger(),
	}
}
