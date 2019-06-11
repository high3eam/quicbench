package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	quic "github.com/lucas-clemente/quic-go"
	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/mybench/bench"
)

var goroutines int = 2
var duration int = 10 //seconds
var statsAggregator chan *bench.Ret
var testUrl string
var method string = "GET"
var runType string = "quic"
var requestNum int = 5
var keepAlive bool = true

func init() {
	flag.IntVar(&goroutines, "c", 10, "Number of goroutines to use (concurrent connections)")
	flag.IntVar(&requestNum, "r", 0, "Number of every goroutine send request")
	flag.IntVar(&duration, "d", 10, "Duration of test in seconds")
	flag.StringVar(&testUrl, "u", "http://www.baidu.com", "url")
	flag.StringVar(&method, "M", "GET", "HTTP method")
	flag.StringVar(&runType, "rt", "http", "http/quic")
	flag.BoolVar(&keepAlive, "k", true, "keepalive")
}

func getRawHttpClient() (client *http.Client) {
	transport := &http.Transport{
		DisableCompression:    false,
		DisableKeepAlives:     !keepAlive,
		ResponseHeaderTimeout: time.Millisecond * time.Duration(1000),
	}
	client = &http.Client{
		Transport: transport,
	}
	return
}

func getQuickHttpClient() (client *http.Client) {
	t := h2quic.RoundTripper{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		QuicConfig: &quic.Config{KeepAlive: true, Versions: []quic.VersionNumber{quic.VersionGQUIC44}},
	}
	client = &http.Client{
		Transport: &t,
	}
	return
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU() + goroutines)
	flag.Parse()
	if requestNum > 0 {
		fmt.Printf("%v Running %v request test @ %v\n  %v goroutine(s) running concurrently\n", runType, requestNum, testUrl, goroutines)
	} else {
		fmt.Printf("%v Running %vs test @ %v\n  %v goroutine(s) running concurrently\n", runType, duration, testUrl, goroutines)

	}

	sigChan := make(chan os.Signal, 1)
	statsAggregator = make(chan *bench.Ret, goroutines)
	signal.Notify(sigChan, os.Interrupt)
	b := bench.NewBench(statsAggregator, float64(duration), requestNum, method, testUrl, keepAlive)
	if runType == "http" {
		client := getRawHttpClient()
		for i := 0; i < goroutines; i++ {
			go b.RunSingleLoadSession(client)
		}
	} else {
		for i := 0; i < goroutines; i++ {
			go b.RunSingleLoadSession(getQuickHttpClient())
		}
	}

	aggStats := bench.Ret{MinRequestTime: time.Minute}
	responders := 0
	for responders < goroutines {
		select {
		case <-sigChan:
			b.Stop()
			fmt.Printf("stopping...\n")
		case stats := <-statsAggregator:
			aggStats.NumErrs += stats.NumErrs
			aggStats.NumRequests += stats.NumRequests
			aggStats.TotRespSize += stats.TotRespSize
			aggStats.TotDuration += stats.TotDuration
			aggStats.MaxRequestTime = bench.MaxDuration(aggStats.MaxRequestTime, stats.MaxRequestTime)
			aggStats.MinRequestTime = bench.MinDuration(aggStats.MinRequestTime, stats.MinRequestTime)
			responders++
		}
	}

	avgThreadDur := aggStats.TotDuration / time.Duration(responders) //need to average the aggregated duration

	reqRate := float64(aggStats.NumRequests) / avgThreadDur.Seconds()
	avgReqTime := aggStats.TotDuration / time.Duration(aggStats.NumRequests)
	bytesRate := float64(aggStats.TotRespSize) / avgThreadDur.Seconds()
	fmt.Printf("%v requests in %v, %v read\n", aggStats.NumRequests, avgThreadDur, float64(aggStats.TotRespSize))
	fmt.Printf("Requests/sec:\t\t%.2f\nTransfer/sec:\t\t%v\nAvg Req Time:\t\t%v\n", reqRate, float64(bytesRate), avgReqTime)
	fmt.Printf("Fastest Request:\t%v\n", aggStats.MinRequestTime)
	fmt.Printf("Slowest Request:\t%v\n", aggStats.MaxRequestTime)
	fmt.Printf("Number of Errors:\t%v\n", aggStats.NumErrs)
}
