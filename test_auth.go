package main

import (
	"fmt"
	"github.com/verssache/chatgpt-creator/internal/email"
	"github.com/verssache/chatgpt-creator/internal/register"
	"sync"
)

func main() {
	var printMu sync.Mutex
	var fileMu sync.Mutex
	client, err := register.NewClient("", "1", 1, &printMu, &fileMu)
	if err != nil {
		panic(err)
	}

	emailAddr := "d.i.j.e.alo.k.74@gmail.com"
	password := "TestPassword123!" // It will use whatever was registered, or fail

	imapCfg := &email.GmailIMAPConfig{
		Email:       "dijealok74@gmail.com",
		AppPassword: "gvqr reyh gadc kiqk",
	}

	fmt.Println("Running test login flow for:", emailAddr)
	
	res, err := client.RunLogin(emailAddr, password, nil, imapCfg)
	if err != nil {
		fmt.Printf("RunLogin failed with error: %v\n", err)
	} else if res != nil {
		fmt.Printf("RunLogin succeeded! Token: %s\n", res.AccessToken)
	} else {
		fmt.Println("RunLogin succeeded! (No K12 workspace requested, so no token object returned)")
	}
}
