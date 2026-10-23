package ipmeta

import (
	"fmt"
	"strings"

	ipUtils "myblogx/utils/user_info"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

var (
	ipv4Searcher *xdb.Searcher
	ipv6Searcher *xdb.Searcher
)

func Init(ipv4Path, ipv6Path string) error {
	ipv4, err := xdb.NewWithFileOnly(xdb.IPv4, ipv4Path)
	if err != nil {
		return fmt.Errorf("ip2region_v4.xdb 加载失败: %w", err)
	}

	ipv6, err := xdb.NewWithFileOnly(xdb.IPv6, ipv6Path)
	if err != nil {
		ipv4.Close()
		return fmt.Errorf("ip2region_v6.xdb 加载失败: %w", err)
	}

	if ipv4Searcher != nil {
		ipv4Searcher.Close()
	}
	if ipv6Searcher != nil {
		ipv6Searcher.Close()
	}

	ipv4Searcher = ipv4
	ipv6Searcher = ipv6
	return nil
}

func GetAddr(ip string) string {
	ipType := ipUtils.IpType(ip)
	if ipType == "1" {
		return "未知地址"
	}

	if ipUtils.IsLocalIP(ip, ipType) {
		return "内网地址"
	}

	var (
		region string
		err    error
	)

	switch ipType {
	case "ipv4":
		if ipv4Searcher == nil {
			return "未知地址"
		}
		region, err = ipv4Searcher.SearchByStr(ip)
	case "ipv6":
		if ipv6Searcher == nil {
			return "未知地址"
		}
		region, err = ipv6Searcher.SearchByStr(ip)
	default:
		return "未知地址"
	}

	if err != nil || region == "" {
		return "未知地址"
	}

	addrList := strings.Split(region, "|")
	if len(addrList) < 4 {
		return "未知地址"
	}

	country := addrList[0]
	province := addrList[1]
	city := addrList[2]

	if country == "中国" {
		if province != "0" && city != "0" {
			return fmt.Sprintf("%s·%s", province, city)
		}
	} else {
		if country != "0" && province != "0" {
			return fmt.Sprintf("%s·%s", country, province)
		}
		if country != "0" {
			return country
		}
	}

	return "未知地址"
}
