package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("WEBPUSH_PUBLIC_KEY=%s\nWEBPUSH_PRIVATE_KEY=%s\n", publicKey, privateKey)
}
