package main

import (
	"fmt"

	"launchpad.net/account-polld/accounts"
)

func main() {
	for data := range accounts.NewWatcher().C {
		if data.Error != nil {
			fmt.Println("Failed to authenticate account", data.AccountId, ":", data.Error)
		} else {
			fmt.Printf("%#v\n", data)
		}
	}
}
