package main

import "fmt"

var (
	AppVersions = [3]uint32{0, 1, 0}
	AppVersion  = fmt.Sprintf("v%d.%d.%d", AppVersions[0], AppVersions[1], AppVersions[2])
	AppPort     = "4001"
	AppOs       = "YourAppName"
)
