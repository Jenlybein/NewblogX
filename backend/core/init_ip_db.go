// IP数据库初始化

package core

import (
	"myblogx/utils/ipmeta"

	"github.com/sirupsen/logrus"
)

func InitIPDB(logger *logrus.Logger) {
	var dbIPv4 = "resources/ipbase/ip2region_v4.xdb"
	var dbIPv6 = "resources/ipbase/ip2region_v6.xdb"

	if err := ipmeta.Init(dbIPv4, dbIPv6); err != nil {
		logger.Fatalf("IP数据库初始化失败:%s", err)
	}
}

func GetIpAddr(ip string) (addr string) {
	return ipmeta.GetAddr(ip)
}
