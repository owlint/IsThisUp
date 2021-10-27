package main

import (
	"bytes"
	"crypto/tls"
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

func testSSLCert(websiteUrl string, validDays time.Duration) (bool, string) {
	u, err := url.Parse(websiteUrl)
	if err != nil {
		return false, "Unable to parse site url"
	}

	conn, err := tls.Dial("tcp", u.Host+":443", nil)
	if err != nil {
		return false, "Server doesn't support SSL certificate err: " + err.Error()
	}

	err = conn.VerifyHostname(u.Host)
	if err != nil {
		return false, "Hostname doesn't match with certificate: " + err.Error()
	}
	expiry := conn.ConnectionState().PeerCertificates[0].NotAfter

	dateCheck := time.Now()
	dateCheck = dateCheck.Add(validDays)

	if expiry.Before(dateCheck) {
		return false, "Certificate expire too soon"
	}

	return true, "ok"
}

func testWebsite(websiteUrl string, requestTimeout time.Duration, maxRetry int, retryTimeout time.Duration, validDays int) bool {
	client := &http.Client{
		Timeout: time.Second * requestTimeout,
	}

	success := false
	retry := 0

	for !success && retry < maxRetry {
		if strings.HasPrefix(websiteUrl, "https://") {
			ok, message := testSSLCert(websiteUrl, time.Hour*24*time.Duration(validDays))
			if !ok {
				retry += 1
				log.Printf("SSL check of %s failed. Retry %d of %d", websiteUrl, retry, maxRetry)
				log.Printf(message)
				time.Sleep(retryTimeout * time.Second)
				continue
			}
		}

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

	validDaysString, haveValue := os.LookupEnv("SSL_DAYS_LIMIT")
	if !haveValue {
		log.Fatalln("No SSL_DAYS_LIMIT env variable. Quitting...")
	}

	validDays, err := strconv.Atoi(validDaysString)
	if err != nil {
		log.Fatalln("SSL_DAYS_LIMIT is not valid int. Quitting...")
	}

	for {
		isConnectedToInternet := isConnectedToInternet()
		if !isConnectedToInternet {
			log.Fatalln("Cannot connect to internet. Quitting...")
		}

		isUp := testWebsite(websiteUrl, time.Duration(timeout), retry, time.Duration(retryTimeout), validDays)

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
