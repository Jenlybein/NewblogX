package core_test

import (
	"myblogx/conf"
	"myblogx/core"
	"myblogx/test/testutil"
	"testing"
)

func TestInitMySQLESDisabled(t *testing.T) {
	testutil.InitGlobals()
	testutil.SetConfig(&conf.Config{
		River: conf.River{
			Enabled: false,
		},
	})

	core.InitMySQLES(core.MySQLESDeps{
		RiverConfig: testutil.Config().River,
		Logger:      testutil.Logger(),
	})
}
