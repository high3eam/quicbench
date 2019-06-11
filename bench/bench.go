package bench

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	USER_AGENT = "migu-quick-client"
)

type Ret struct {
	TotRespSize    int64
	MinRequestTime time.Duration
	MaxRequestTime time.Duration
	NumRequests    int
	NumErrs        int
	TotDuration    time.Duration
}

type Bench struct {
	statsAggregator chan *Ret
	duration        float64
	requestNum      int
	keepAlive       bool
	method          string
	url             string
	interrupted     int32
}

func MaxDuration(d1 time.Duration, d2 time.Duration) time.Duration {
	if d1 > d2 {
		return d1
	} else {
		return d2
	}
}

func MinDuration(d1 time.Duration, d2 time.Duration) time.Duration {
	if d1 < d2 {
		return d1
	} else {
		return d2
	}
}

func EstimateHttpHeadersSize(headers http.Header) (result int64) {
	result = 0

	for k, v := range headers {
		result += int64(len(k) + len(": \r\n"))
		for _, s := range v {
			result += int64(len(s))
		}
	}

	result += int64(len("\r\n"))

	return result
}

func DoRequest(client *http.Client, method, url string, keepAlive bool) (respSize int, duration time.Duration) {
	var buf io.Reader
	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		fmt.Println(err)
		return
	}

	if keepAlive {
		req.Header.Add("Connection", "keep-alive")
	} else {
		req.Header.Add("Connection", "close")
	}

	req.Header.Add("User-Agent", USER_AGENT)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}

	if resp == nil {
		fmt.Println("empty response")
		return
	}

	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	body, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		duration = time.Since(start)
		respSize = len(body) + int(EstimateHttpHeadersSize(resp.Header))
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respSize = int(resp.ContentLength) + int(EstimateHttpHeadersSize(resp.Header))
	} else if err != nil {
		fmt.Println(err)
	}
	return
}

func (b *Bench) RunSingleLoadSession(client *http.Client) {
	ret := &Ret{}
	if b.requestNum == 0 {
		start := time.Now()
		for time.Since(start).Seconds() <= b.duration && atomic.LoadInt32(&b.interrupted) == 0 {
			respSize, reqDur := DoRequest(client, b.method, b.url, b.keepAlive)
			if respSize > 0 {
				ret.TotRespSize += int64(respSize)
				ret.TotDuration += reqDur
				ret.MaxRequestTime = MaxDuration(reqDur, ret.MaxRequestTime)
				ret.MinRequestTime = MinDuration(reqDur, ret.MinRequestTime)
				ret.NumRequests++
			} else {
				ret.NumErrs++
			}

		}
	} else {
		for i := 0; i < b.requestNum; i++ {
			respSize, reqDur := DoRequest(client, b.method, b.url, b.keepAlive)
			if respSize > 0 {
				ret.TotRespSize += int64(respSize)
				ret.TotDuration += reqDur
				ret.MaxRequestTime = MaxDuration(reqDur, ret.MaxRequestTime)
				ret.MinRequestTime = MinDuration(reqDur, ret.MinRequestTime)
				ret.NumRequests++
			} else {
				ret.NumErrs++
			}
		}
	}

	b.statsAggregator <- ret
}

func (b *Bench) Stop() {
	atomic.StoreInt32(&b.interrupted, 1)
}

func NewBench(statsAggregator chan *Ret, duration float64, requestNum int, method, url string, keepAlive bool) (b *Bench) {
	bench := Bench{statsAggregator: statsAggregator, duration: duration, requestNum: requestNum, method: method, url: url, keepAlive: keepAlive}
	return &bench
}
