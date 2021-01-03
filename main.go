package main

import "github.com/NateScarlet/pixiv/pkg/client"

func main() {

	var c = &client.Client{}
	c.BypassSNIBlocking()
	c.SetDefaultHeader("User-Agent", client.DefaultUserAgent)
	c.SetPHPSESSID("789096_Rf8DAAmgdmqQHKf04vhPkrP6GYBuO7Fr")
	result, _ := c.IsLoggedIn()
	println(result)
}
