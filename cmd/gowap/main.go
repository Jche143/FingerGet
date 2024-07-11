package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"scanner/pkg/core"
)

var url string

func init() {
	flag.StringVar(&url, "u", "", "url")
}

func main() {
	flag.Parse()
	wapp, _ := core.Init("pkg/finger/app.json", false)

	httpdata, err := core.SendRequest(wapp, url)

	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println("url:", httpdata.Url)

	res, err := wapp.Analyze(httpdata)
	if err != nil {
		log.Fatal(err)
	}
	r, _ := json.MarshalIndent(res, "", "\t")
	fmt.Println(string(r))
}
