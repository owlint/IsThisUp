package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sparrc/go-ping"
)

func isConnectedToInternet() bool {
	log.Println("Test internet connection")

	pinger, err := ping.NewPinger("www.google.com")
	pinger.SetPrivileged(true)
	if err != nil {
		log.Fatalln(err)
	}

	pinger.Count = 3
	pinger.Run()                 // blocks until finished
	stats := pinger.Statistics() // get send/receive/rtt stats

	return stats.PacketsRecv > 0
}

func testWebsite(websiteUrl string, requestTimeout time.Duration, maxRetry int, retryTimeout time.Duration) bool {
	client := &http.Client{
		Timeout: time.Second * requestTimeout,
	}

	success := false
	retry := 0

	for !success && retry < maxRetry {
		req, _ := http.NewRequest("GET", websiteUrl, nil)
		resp, err := client.Do(req)

		if err != nil {
			log.Printf("Error performing request : %v", err)
		} else {
			success = resp.StatusCode < 400
		}

		defer resp.Body.Close()

		if !success {
			retry += 1
			log.Printf("Check of %s failed. Retry %d of %d", websiteUrl, retry, maxRetry)
			time.Sleep(retryTimeout * time.Second)
		}
	}

	return success
}

func websiteURLToAlias(websiteUrl string) string {
	u, err := url.Parse(websiteUrl)
	if err != nil {
		log.Fatalln(err)
	}
	return strings.ReplaceAll(u.Host, ".", "-")
}

func alertPager(websiteUrl string, apiKey string) {
	message := fmt.Sprintf("%s is not responding", websiteUrl)
	values := map[string]interface{}{
		"routing_key":  apiKey,
		"event_action": "trigger",
		"dedup_key":    websiteURLToAlias(websiteUrl),
		"payload": map[string]string{
			"summary":  message,
			"source":   "isThisUp",
			"severity": "critical",
		},
	}
	jsonValue, _ := json.Marshal(values)

	client := &http.Client{}
	req, errreq := http.NewRequest("POST", "https://events.pagerduty.com/v2/enqueue", bytes.NewBuffer(jsonValue))
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)

	if err != nil || errreq != nil || resp.StatusCode > 400 {
		log.Fatalln("Cannot alert with PagerDuty. Quitting...")
	}

	defer resp.Body.Close()
}

func alertOpsGenie(websiteUrl string, apiKey string) {
	message := fmt.Sprintf("%s is not responding", websiteUrl)
	values := map[string]string{"message": message, "priority": "P1", "alias": websiteURLToAlias(websiteUrl)}
	jsonValue, _ := json.Marshal(values)

	client := &http.Client{}
	req, errreq := http.NewRequest("POST", "https://api.eu.opsgenie.com/v2/alerts", bytes.NewBuffer(jsonValue))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("GenieKey %s", apiKey))

	resp, err := client.Do(req)

	if err != nil || errreq != nil || resp.StatusCode > 400 {
		log.Fatalln("Cannot alert with OpsGenie. Quitting...")
	}

	defer resp.Body.Close()
}

func main() {
	websiteUrl, haveValue := os.LookupEnv("URL")
	if !haveValue {
		log.Fatalln("No URL env variable. Quitting...")
	}

	plateform, haveValue := os.LookupEnv("PLATEFORM")
	if !haveValue {
		log.Fatalln("No PLATEFORM env variable. Quitting...")
	}

	if plateform != "pagerduty" && plateform != "opsgenie" {
		log.Fatalln("Invalid PLATEFORM. Quitting...")
	}

	apiKey, haveValue := os.LookupEnv("API_KEY")
	if !haveValue {
		log.Fatalln("No API_KEY env variable. Quitting...")
	}

	sleepingTimeString, haveValue := os.LookupEnv("SLEEP")
	if !haveValue {
		log.Fatalln("No SLEEP env variable. Quitting...")
	}

	sleepingTime, err := strconv.Atoi(sleepingTimeString)
	if err != nil {
		log.Fatalln("SLEEP is not valid int. Quitting...")
	}

	timeoutString, haveValue := os.LookupEnv("TIMEOUT")
	if !haveValue {
		log.Fatalln("No TIMEOUT env variable. Quitting...")
	}

	timeout, err := strconv.Atoi(timeoutString)
	if err != nil {
		log.Fatalln("TIMEOUT is not valid int. Quitting...")
	}

	retryString, haveValue := os.LookupEnv("RETRY")
	if !haveValue {
		log.Fatalln("No RETRY env variable. Quitting...")
	}

	retry, err := strconv.Atoi(retryString)
	if err != nil {
		log.Fatalln("RETRY is not valid int. Quitting...")
	}

	retryTimeoutString, haveValue := os.LookupEnv("RETRY_TIMEOUT")
	if !haveValue {
		log.Fatalln("No RETRY_TIMEOUT env variable. Quitting...")
	}

	retryTimeout, err := strconv.Atoi(retryTimeoutString)
	if err != nil {
		log.Fatalln("RETRY_TIMEOUT is not valid int. Quitting...")
	}

	for {
		isConnectedToInternet := isConnectedToInternet()
		if !isConnectedToInternet {
			log.Fatalln("Cannot connect to internet. Quitting...")
		}

		isUp := testWebsite(websiteUrl, time.Duration(timeout), retry, time.Duration(retryTimeout))

		if isUp {
			log.Printf("%s is up", websiteUrl)
		} else {
			log.Printf("%s is down", websiteUrl)
		}

		if !isUp && isConnectedToInternet {
			switch plateform {
			case "pagerduty":
				alertPager(websiteUrl, apiKey)
			case "opsgenie":
				alertOpsGenie(websiteUrl, apiKey)
			}
		}

		time.Sleep(time.Duration(sleepingTime) * time.Second)
	}

}
