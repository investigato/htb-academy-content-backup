package main

import (
	"flag"
	"fmt"
	"os"
)

type Args struct {
	moduleUrl        string
	cookies          string
	saveImages       bool
	saveWalkthroughs bool
	retryFailed      bool
	proxy						 bool
}

func getArguments() Args {
	var mFlag = flag.String("m", "", "Academy Module URL to the first page.")
	var cFlag = flag.String("c", "", "(REQUIRED unless -retry-failed) Academy Cookies for authorization.")
	var imgFlag = flag.Bool("save-images", true, "Save images locally rather than referencing the URL location.")
	var walkthroughFlag = flag.Bool("save-walkthroughs", true, "Save walkthroughs locally.")
	var retryFlag = flag.Bool("retry-failed", false, "Retry failed image downloads recorded in tracker.json.")
	flag.Parse()
	var proxyFlag = flag.Bool("proxy", false, "Route requests through proxy at http://127.0.0.1:8080 for debugging with something like Burp Suite.")

	arg := Args{
		moduleUrl:        *mFlag,
		cookies:          *cFlag,
		saveImages:       *imgFlag,
		saveWalkthroughs: *walkthroughFlag,
		retryFailed:      *retryFlag,
		proxy:						*proxyFlag,
	}

	// must have cookies, unless JUST retrying failed images (which might not require either since it goes direct to the CDN).
	if arg.cookies == "" && !arg.retryFailed {
		fmt.Println("Missing required argument: -c (cookies). Use -h for help.")
		os.Exit(1)
	}

	return arg
}
